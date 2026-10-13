package advancedreality

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func nativePort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
func nativeJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal("fixture encoding")
	}
	return b
}

type nativeDiagnostics struct {
	mu     sync.Mutex
	counts map[string]int
}

func (d *nativeDiagnostics) Write(b []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := strings.ToLower(string(b))
	for _, code := range []string{"certificate", "x509", "tls", "reality", "invalid padding", "bad status", "timeout", "eof", "refused", "permission denied", "operation not permitted", "failed to dial", "failed to read", "accepted", "freedom", "vless", "xhttp", "http2", "context canceled", "invalid user", "invalid request", "blocked target"} {
		if strings.Contains(s, code) {
			d.counts[code]++
		}
	}
	return len(b), nil
}

// Diagnostics are intentionally discarded: some core validators echo secrets.
func startNative(t *testing.T, binary string, raw []byte, port int) func() {
	t.Helper()
	diagnostics := os.Getenv("ADVANCED_REALITY_TEST_DIAGNOSTICS") == "1"
	if diagnostics {
		raw = bytes.Replace(raw, []byte(`"loglevel":"none"`), []byte(`"loglevel":"info"`), 1)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("fixture write")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if e := exec.CommandContext(ctx, binary, "run", "-test", "-config", path).Run(); e != nil {
		t.Fatal("native config validation failed; diagnostics redacted")
	}
	cmd := exec.Command(binary, "run", "-config", path)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	diag := &nativeDiagnostics{counts: map[string]int{}}
	if diagnostics {
		cmd.Stdout = diag
		cmd.Stderr = diag
	}
	if cmd.Start() != nil {
		t.Fatal("native start failed")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		if diagnostics {
			diag.mu.Lock()
			t.Logf("redacted diagnostic categories: %v", diag.counts)
			diag.mu.Unlock()
		}
	}
	t.Cleanup(stop)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			stopped = true
			t.Fatal("native process exited before readiness")
		default:
		}
		c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 100*time.Millisecond)
		if e == nil {
			c.Close()
			return stop
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("native listener deadline")
	return stop
}

func nativeCertificate(t *testing.T) (string, string) {
	t.Helper()
	// Reuse Go's localhost fixture certificate/key: this is an explicitly
	// trusted isolated CA, never an allowInsecure setting or live certificate.
	s := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer s.Close()
	cert := s.TLS.Certificates[0]
	der, e := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if e != nil {
		t.Fatal(e)
	}
	d := t.TempDir()
	cf, kf := filepath.Join(d, "ca.pem"), filepath.Join(d, "key.pem")
	if os.WriteFile(cf, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), 0600) != nil || os.WriteFile(kf, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0600) != nil {
		t.Fatal("certificate fixture write")
	}
	return cf, kf
}

func TestNativeXrayTLSXHTTPAndRealityRollback(t *testing.T) {
	binary := os.Getenv("ADVANCED_REALITY_TEST_XRAY")
	if binary == "" {
		t.Skip("BLOCKED: exact local official core binary not supplied")
	}
	out, e := exec.Command(binary, "version").Output()
	if e != nil {
		t.Fatal("core identity unavailable")
	}
	parts := strings.Fields(string(out))
	if len(parts) < 2 || parts[0] != "Xray" {
		t.Fatal("unrecognized core identity")
	}
	version := parts[1]
	if _, e = Inspect([]byte(`{}`), []byte(`{}`), version); e != nil {
		t.Fatal(e)
	}
	clientBinary := os.Getenv("ADVANCED_REALITY_TEST_CLIENT_XRAY")
	if clientBinary == "" {
		clientBinary = binary
	}
	clientOut, e := exec.Command(clientBinary, "version").Output()
	if e != nil {
		t.Fatal("client core identity unavailable")
	}
	clientParts := strings.Fields(string(clientOut))
	if len(clientParts) < 2 || clientParts[0] != "Xray" {
		t.Fatal("unrecognized client core identity")
	}
	t.Logf("tested server Xray %s and client Xray %s; not Android/v2rayNG proof", version, clientParts[1])
	payload := bytes.Repeat([]byte("synthetic-are-payload-"), 32768)
	if os.Getenv("ADVANCED_REALITY_TEST_SMALL_PAYLOAD") == "1" {
		payload = payload[:128]
	}
	want := sha256.Sum256(payload)
	var originRequests atomic.Int64
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { originRequests.Add(1); _, _ = w.Write(payload) }))
	defer origin.Close()
	cf, kf := nativeCertificate(t)
	for _, security := range []string{"tls", "reality"} {
		t.Run(security, func(t *testing.T) {
			serverPort, clientPort := nativePort(t), nativePort(t)
			serverStream := map[string]any{"network": "xhttp", "security": security, "xhttpSettings": map[string]any{"path": "/are-test", "mode": "auto"}}
			clientStream := map[string]any{"network": "xhttp", "security": security, "xhttpSettings": map[string]any{"path": "/are-test", "mode": "stream-one"}}
			if security == "tls" {
				serverStream["tlsSettings"] = map[string]any{"minVersion": "1.3", "alpn": []string{"h2"}, "certificates": []any{map[string]any{"certificateFile": cf, "keyFile": kf}}}
				clientStream["tlsSettings"] = map[string]any{"serverName": "example.com", "minVersion": "1.3", "alpn": []string{"h2"}, "certificates": []any{map[string]any{"certificateFile": cf, "usage": "verify"}}}
			} else {
				key, e := ecdh.X25519().GenerateKey(rand.Reader)
				if e != nil {
					t.Fatal(e)
				}
				target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
				target.EnableHTTP2 = true
				target.StartTLS()
				defer target.Close()
				serverStream["realitySettings"] = map[string]any{"dest": strings.TrimPrefix(target.URL, "https://"), "serverNames": []string{"example.com"}, "privateKey": base64.RawURLEncoding.EncodeToString(key.Bytes()), "shortIds": []string{"1a2b3c4d"}}
				clientStream["realitySettings"] = map[string]any{"serverName": "example.com", "fingerprint": "chrome", "publicKey": base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), "shortId": "1a2b3c4d"}
			}
			uuid := "810d360d-98e1-44ab-bd30-f83824601a49"
			server := map[string]any{"log": map[string]any{"loglevel": "none"}, "policy": map[string]any{"levels": map[string]any{"0": map[string]any{"bufferSize": 64, "handshake": 8}}}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": serverPort, "protocol": "vless", "settings": map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": uuid}}}, "streamSettings": serverStream}}, "outbounds": []any{map[string]any{"protocol": "freedom"}}}
			// Newer cores deny private destinations by default. The synthetic origin
			// is deliberately loopback: allow ONLY this fixture's exact port/address,
			// from the initial fixture baseline, never in a live candidate proposal.
			if version == "26.5.9" || version == "26.9.9" || version == "26.10.10" {
				originURL, _ := url.Parse(origin.URL)
				server["outbounds"].([]any)[0].(map[string]any)["settings"] = map[string]any{"finalRules": []any{map[string]any{"action": "allow", "network": "tcp", "ip": []string{"127.0.0.1/32"}, "port": originURL.Port()}}}
			}
			client := map[string]any{"log": map[string]any{"loglevel": "none"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": clientPort, "protocol": "http", "settings": map[string]any{}}}, "outbounds": []any{map[string]any{"protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": "127.0.0.1", "port": serverPort, "users": []any{map[string]any{"id": uuid, "encryption": "none"}}}}}, "streamSettings": clientStream}}}
			before := nativeJSON(t, server)
			after := bytes.Replace(before, []byte(`"bufferSize":64`), []byte(`"bufferSize":128`), 1)
			if _, e = Inspect(before, after, version); e != nil {
				t.Fatal(e)
			}
			lab := Lab{Dir: filepath.Join(t.TempDir(), "snapshots")}
			initial, e := lab.Init(before, version)
			if e != nil {
				t.Fatal(e)
			}
			var staged Receipt
			for _, phase := range []struct {
				name string
				raw  []byte
			}{{"baseline", before}, {"candidate", after}, {"rollback", before}} {
				t.Run(phase.name, func(t *testing.T) {
					if phase.name == "candidate" {
						staged, e = lab.Stage(after, initial.CurrentSHA256, "native-stage")
						if e != nil {
							t.Fatal(e)
						}
					}
					if phase.name == "rollback" {
						rolled, e := lab.Rollback(staged.CurrentSHA256, "native-rollback")
						if e != nil || rolled.CurrentSHA256 != initial.CurrentSHA256 {
							t.Fatal("lab rollback failed")
						}
					}
					raw, e := lab.CurrentConfig()
					if e != nil || !bytes.Equal(raw, phase.raw) {
						t.Fatal("snapshot export mismatch")
					}
					stopServer := startNative(t, binary, raw, serverPort)
					defer stopServer()
					stopClient := startNative(t, clientBinary, nativeJSON(t, client), clientPort)
					defer stopClient()
					proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", clientPort))
					tr := &http.Transport{Proxy: http.ProxyURL(proxy)}
					defer tr.CloseIdleConnections()
					hc := &http.Client{Transport: tr, Timeout: 10 * time.Second}
					prior := originRequests.Load()
					resp, e := hc.Get(origin.URL)
					if e != nil {
						t.Fatalf("tunnel request failed; origin reached: %t; details redacted", originRequests.Load() > prior)
					}
					defer resp.Body.Close()
					body, e := io.ReadAll(io.LimitReader(resp.Body, int64(len(payload)+1)))
					if e != nil || resp.StatusCode != 200 || sha256.Sum256(body) != want {
						t.Fatal("tunnel payload integrity failed")
					}
					t.Logf("%s %s: %d synthetic bytes preserved, sha256 %s", security, phase.name, len(body), hex.EncodeToString(want[:]))
				})
			}
			t.Run("reject_invalid_authentication", func(t *testing.T) {
				stopServer := startNative(t, binary, before, serverPort)
				defer stopServer()
				if security == "tls" {
					clientStream["tlsSettings"].(map[string]any)["serverName"] = "wrong-name.invalid"
				} else {
					clientStream["realitySettings"].(map[string]any)["shortId"] = "ffffffff"
				}
				stopClient := startNative(t, clientBinary, nativeJSON(t, client), clientPort)
				defer stopClient()
				proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", clientPort))
				tr := &http.Transport{Proxy: http.ProxyURL(proxy)}
				defer tr.CloseIdleConnections()
				hc := &http.Client{Transport: tr, Timeout: 4 * time.Second}
				resp, e := hc.Get(origin.URL)
				if e == nil {
					defer resp.Body.Close()
					body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(len(payload)+1)))
					if resp.StatusCode == 200 && sha256.Sum256(body) == want {
						t.Fatal("invalid authentication accepted")
					}
				}
			})
			if e = lab.Remove(initial.CurrentSHA256); e != nil {
				t.Fatal(e)
			}
		})
	}
}
