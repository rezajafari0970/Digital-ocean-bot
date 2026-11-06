package adminapi

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
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
func TestNativeBrowserXrayCompatibility(t *testing.T) {
	core := os.Getenv("DOB_TEST_XRAY")
	if core == "" {
		t.Skip("native Xray fixture requires DOB_TEST_XRAY")
	}
	_, b, h, calls, secure := nativeFixture(t)
	u, _ := url.Parse(h.URL)
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
