package adminapi

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/vultr"
	"golang.org/x/net/websocket"
)

func nativeFixture(t *testing.T) (*Server, *nativeBrowserSession, *httptest.Server, *atomic.Int64, *httptest.Server) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	key := [16]byte{0x11, 0x11, 0x11, 0x11, 0x22, 0x22, 0x43, 0x33, 0x84, 0x44, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55}
	calls := new(atomic.Int64)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Native-Fixture", "yes")
		if r.URL.Path == "/close" {
			w.Header().Set("Connection", "close")
		}
		if r.URL.Path == "/large" {
			w.Write([]byte(strings.Repeat("native body ", 20000)))
			return
		}
		w.Write([]byte("original site response"))
	})
	up := httptest.NewServer(handler)
	secure := httptest.NewTLSServer(handler)
	t.Cleanup(secure.Close)
	t.Cleanup(up.Close)
	address := strings.TrimPrefix(up.URL, "http://")
	b := &nativeBrowserSession{ctx: ctx, cancel: cancel, owner: "owner", login: "login", provider: "vultr", expires: time.Now().Add(time.Minute), credential: key, route: &app.OnboardingRoute{Country: "AR", IP: "198.51.100.1", Close: func() {}, Dial: func(ctx context.Context, dest string) (net.Conn, error) {
		target := address
		if dest == "example.com:443" {
			target = strings.TrimPrefix(secure.URL, "https://")
		} else if dest != "example.com:80" {
			return nil, context.Canceled
		}
		calls.Add(1)
		return (&net.Dialer{}).DialContext(ctx, "tcp", target)
	}}}
	s := &Server{NativeBrowsers: map[string]*nativeBrowserSession{strings.Repeat("a", 48): b}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/provider-browser/native-stream/{id}", s.nativeBrowserStream)
	h := httptest.NewTLSServer(mux)
	t.Cleanup(h.Close)
	t.Cleanup(func() { b.close("test_complete") })
	return s, b, h, calls, secure
}
func TestNativeBrowserOwnershipAndCredentials(t *testing.T) {
	s, b, h, calls, _ := nativeFixture(t)
	for _, p := range []auth.Principal{{UserID: "other", SessionID: "login", Role: auth.Admin}, {UserID: "owner", SessionID: "other", Role: auth.Admin}} {
		r := httptest.NewRequest("GET", "/", nil)
		r.SetPathValue("id", strings.Repeat("a", 48))
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, p))
		w := httptest.NewRecorder()
		s.nativeBrowserStatus(w, r)
		if w.Code != 404 {
			t.Fatal("status crossed owner/login boundary", w.Code)
		}
		w = httptest.NewRecorder()
		s.stopNativeBrowser(w, r)
		if b.closed {
			t.Fatal("another login revoked connection")
		}
	}
	endpoint := "wss" + strings.TrimPrefix(h.URL, "https") + "/api/v1/provider-browser/native-stream/" + strings.Repeat("a", 48)
	config, e := websocket.NewConfig(endpoint, h.URL)
	if e != nil {
		t.Fatal(e)
	}
	config.TlsConfig = &tls.Config{InsecureSkipVerify: true}
	if conn, e := websocket.DialConfig(config); e == nil {
		conn.Close()
		t.Fatal("browser Origin accepted")
	}
	config.Origin = &url.URL{}
	conn, e := websocket.DialConfig(config)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	conn.PayloadType = websocket.BinaryFrame
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	conn.Write(make([]byte, 18))
	var result [2]byte
	if _, e = conn.Read(result[:]); e == nil {
		t.Fatal("invalid credential accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("unauthorized egress")
	}
	s.closeNativeBrowsers("login")
	if !b.closed {
		t.Fatal("logout did not revoke")
	}
	if conn, e = websocket.DialConfig(config); e == nil {
		conn.Close()
		t.Fatal("revoked session upgraded")
	}
	registry := providers.NewRegistry()
	registry.Register(vultr.Factory{})
	s = &Server{Container: app.Container{Providers: registry}}
	for _, body := range []string{`{"provider":"vultr"}`, `{"provider":"vultr","proxy_id":"p","url":"https://arbitrary.invalid/"}`, `{"provider":"vultr","proxy_id":"p","proxy_countries":["bad"]}`} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{UserID: "owner", SessionID: "login", Role: auth.Admin}))
		w := httptest.NewRecorder()
		s.startNativeBrowser(w, r)
		if w.Code != 400 {
			t.Fatal("invalid native start accepted", w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cacheable credentials")
		}
	}
}
func TestNativeBrowserXrayCompatibility(t *testing.T)         { nativeXrayCompatibility(t, false) }
func TestNativeBrowserNginxGatewayCompatibility(t *testing.T) { nativeXrayCompatibility(t, true) }
func nativeXrayCompatibility(t *testing.T, gateway bool) {
	core := os.Getenv("DOB_TEST_XRAY")
	if core == "" {
		t.Skip("native Xray fixture requires DOB_TEST_XRAY")
	}
	_, b, h, calls, secure := nativeFixture(t)
	endpoint := h.URL
	if gateway {
		endpoint = nativeNginxGateway(t, h)
	}
	u, _ := url.Parse(endpoint)
	host, portText, _ := net.SplitHostPort(u.Host)
	var port int
	json.Unmarshal([]byte(portText), &port)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	local := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	pin := sha256.Sum256(h.Certificate().Raw)
	config := map[string]any{"log": map[string]any{"loglevel": "debug"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": local, "protocol": "socks", "settings": map[string]any{"auth": "noauth"}}}, "outbounds": []any{map[string]any{"protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": host, "port": port, "users": []any{map[string]any{"id": "11111111-2222-4333-8444-555555555555", "encryption": "none"}}}}}, "streamSettings": map[string]any{"network": "ws", "security": "tls", "tlsSettings": map[string]any{"serverName": host, "pinnedPeerCertSha256": hex.EncodeToString(pin[:])}, "wsSettings": map[string]any{"path": "/api/v1/provider-browser/native-stream/" + strings.Repeat("a", 48)}}, "mux": map[string]any{"enabled": false}}}}
	// Only this disposable httptest certificate is pinned. The exported user
	// profile is separately checked to require normal certificate verification.
	raw, _ := json.Marshal(config)
	file := filepath.Join(t.TempDir(), "client.json")
	os.WriteFile(file, raw, 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, core, "run", "-c", file)
	logFile, err := os.Create(filepath.Join(filepath.Dir(file), "core.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	proxy, _ := url.Parse("socks5://127.0.0.1:" + intText(local))
	roots := x509.NewCertPool()
	roots.AddCert(secure.Certificate())
	tr := &http.Transport{Proxy: http.ProxyURL(proxy), TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "example.com"}}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	var response *http.Response
	for i := 0; i < 30; i++ {
		response, e = client.Get("http://example.com/native-fixture")
		if e == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if e != nil {
		output, _ := os.ReadFile(logFile.Name())
		t.Fatal("real Xray could not use native gateway", e, string(output))
	}
	payload, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(payload) != "original site response" || response.Header.Get("X-Native-Fixture") != "yes" || calls.Load() < 1 {
		output, _ := os.ReadFile(logFile.Name())
		t.Fatal("native payload lost", response.Status, string(payload), "dials", calls.Load(), string(output))
	}
	for range 10 {
		response, e = client.Get("http://example.com/close")
		if e != nil {
			t.Fatal("server close lost response", e)
		}
		payload, e = io.ReadAll(response.Body)
		response.Body.Close()
		if e != nil || string(payload) != "original site response" {
			t.Fatal("server close lost payload", string(payload), e)
		}
	}
	for _, address := range []string{"http://example.com/large", "https://example.com/", "https://example.com/large", "https://example.com/close"} {
		response, e = client.Get(address)
		if e != nil {
			t.Fatal("native HTTP/TLS fixture", address, e)
		}
		payload, e = io.ReadAll(response.Body)
		response.Body.Close()
		expected := "original site response"
		if strings.HasSuffix(address, "/large") {
			expected = strings.Repeat("native body ", 20000)
		}
		if e != nil || string(payload) != expected {
			t.Fatal("native HTTP/TLS payload", address, len(payload), e)
		}
	}
	b.close("ended_by_user")
	tr.CloseIdleConnections()
	if response, e = client.Get("http://example.com/native-fixture"); e == nil {
		response.Body.Close()
		if response.StatusCode < 400 {
			t.Fatal("revoked Xray connection still usable")
		}
	}
	t.Log("NATIVE_XRAY_WSS_ORIGINAL_PAYLOAD_AND_REVOCATION PASS")
}
func intText(n int) string { b, _ := json.Marshal(n); return string(b) }

func nativeNginxGateway(t *testing.T, backend *httptest.Server) string {
	t.Helper()
	binary := os.Getenv("DOB_TEST_NGINX")
	if binary == "" {
		t.Skip("native gateway fixture requires DOB_TEST_NGINX")
	}
	root := t.TempDir()
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: backend.Certificate().Raw})
	key, err := x509.MarshalPKCS8PrivateKey(backend.TLS.Certificates[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "cert.pem"), certificate, 0600)
	os.WriteFile(filepath.Join(root, "key.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	config := "daemon off; master_process off; pid " + root + "/nginx.pid; error_log stderr; events {worker_connections 64;} http {access_log off; client_body_temp_path " + root + "/body; proxy_temp_path " + root + "/proxy; server {listen " + address + " ssl; ssl_certificate " + root + "/cert.pem; ssl_certificate_key " + root + "/key.pem; location / {proxy_pass " + backend.URL + "; proxy_http_version 1.1; proxy_set_header Connection close; proxy_set_header Upgrade \"\"; proxy_set_header X-Forwarded-Proto https;}}}"
	file := filepath.Join(root, "nginx.conf")
	os.WriteFile(file, []byte(config), 0600)
	output, err := os.Create(filepath.Join(root, "nginx.log"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { output.Close() })
	cmd := exec.Command(binary, "-p", root+"/", "-c", file)
	cmd.Stdout = output
	cmd.Stderr = output
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	for range 40 {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return "https://" + address
		}
		time.Sleep(50 * time.Millisecond)
	}
	log, _ := os.ReadFile(output.Name())
	t.Fatal("disposable nginx failed", string(log))
	return ""
}
func TestNativeBrowserGatewayHandshakeBoundary(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "127.0.0.1:2345"
	request.Header.Set("Sec-WebSocket-Key", base64.StdEncoding.EncodeToString([]byte("native-fixture!!")))
	request.Header.Set("Sec-WebSocket-Version", "13")
	request.Header.Set("Connection", "close")
	restored := nativeGatewayWebSocketRequest(request)
	if restored.Header.Get("Upgrade") != "websocket" || restored.Header.Get("Connection") != "Upgrade" || request.Header.Get("Upgrade") != "" {
		t.Fatal("gateway handshake missing or original request mutated")
	}
	for _, mode := range []string{"remote", "origin", "key", "version", "protocol"} {
		candidate := request.Clone(request.Context())
		switch mode {
		case "remote":
			candidate.RemoteAddr = "203.0.113.4:1234"
		case "origin":
			candidate.Header.Set("Origin", "https://untrusted.invalid")
		case "key":
			candidate.Header.Set("Sec-WebSocket-Key", "bad")
		case "version":
			candidate.Header.Set("Sec-WebSocket-Version", "12")
		case "protocol":
			candidate.Header.Set("Upgrade", "other")
		}
		if nativeGatewayWebSocketRequest(candidate).Header.Get("Upgrade") == "websocket" {
			t.Fatal("unexpected reconstruction", mode)
		}
	}
}
