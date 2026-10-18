package configdriver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
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
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
func coreStart(t *testing.T, core string, config any, port int) {
	t.Helper()
	b, _ := json.Marshal(config)
	file := filepath.Join(t.TempDir(), "core.json")
	if e := os.WriteFile(file, b, 0600); e != nil {
		t.Fatal(e)
	}
	if out, e := exec.Command(core, "run", "-test", "-c", file).CombinedOutput(); e != nil {
		t.Fatalf("native config rejected: %s", out)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, core, "run", "-c", file)
	log, e := os.CreateTemp(t.TempDir(), "core-log")
	if e != nil {
		t.Fatal(e)
	}
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Start(); e != nil {
		cancel()
		t.Fatal(e)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
		log.Close()
		if t.Failed() {
			b, _ := os.ReadFile(log.Name())
			t.Log(string(b))
		}
	})
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 50*time.Millisecond)
		if e == nil {
			c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("native listener did not start")
}

// Decode the actual subscription URI independently of the server builder.
func clientFromURI(t *testing.T, uri, cert string) map[string]any {
	t.Helper()
	stream := map[string]any{}
	settings := map[string]any{}
	protocol := ""
	if strings.HasPrefix(uri, "vmess://") {
		b, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "vmess://"))
		if e != nil {
			t.Fatal(e)
		}
		var v map[string]string
		if json.Unmarshal(b, &v) != nil {
			t.Fatal("VMess URI")
		}
		port, _ := strconv.Atoi(v["port"])
		protocol = "vmess"
		settings["vnext"] = []any{map[string]any{"address": v["add"], "port": port, "users": []any{map[string]any{"id": v["id"], "security": v["scy"]}}}}
		stream["network"] = v["net"]
		stream["security"] = "none"
		if v["tls"] == "tls" {
			stream["security"] = "tls"
		}
		if v["net"] == "ws" {
			stream["wsSettings"] = map[string]any{"path": v["path"], "headers": map[string]string{"Host": v["host"]}}
		}
		if v["net"] == "grpc" {
			stream["grpcSettings"] = map[string]any{"serviceName": v["path"]}
		}
	} else {
		u, e := url.Parse(uri)
		if e != nil {
			t.Fatal(e)
		}
		q := u.Query()
		port, _ := strconv.Atoi(u.Port())
		protocol = u.Scheme
		stream["network"] = q.Get("type")
		stream["security"] = q.Get("security")
		switch protocol {
		case "ss":
			protocol = "shadowsocks"
			b, e := base64.RawURLEncoding.DecodeString(u.User.Username())
			if e != nil {
				t.Fatal(e)
			}
			method, password, ok := strings.Cut(string(b), ":")
			if !ok {
				t.Fatal("SS URI")
			}
			settings["servers"] = []any{map[string]any{"address": u.Hostname(), "port": port, "method": method, "password": password}}
			stream["network"] = "tcp"
			stream["security"] = "none"
		case "trojan":
			settings["servers"] = []any{map[string]any{"address": u.Hostname(), "port": port, "password": u.User.Username()}}
		case "vless":
			settings["vnext"] = []any{map[string]any{"address": u.Hostname(), "port": port, "users": []any{map[string]any{"id": u.User.Username(), "encryption": q.Get("encryption"), "flow": q.Get("flow")}}}}
		}
		if q.Get("type") == "ws" {
			stream["wsSettings"] = map[string]any{"path": q.Get("path"), "headers": map[string]string{"Host": q.Get("host")}}
		}
		if q.Get("type") == "grpc" {
			stream["grpcSettings"] = map[string]any{"serviceName": q.Get("serviceName")}
		}
		if q.Get("type") == "xhttp" {
			stream["xhttpSettings"] = map[string]any{"path": q.Get("path"), "host": q.Get("host"), "mode": q.Get("mode")}
		}
		if q.Get("security") == "reality" {
			stream["realitySettings"] = map[string]any{"serverName": q.Get("sni"), "publicKey": q.Get("pbk"), "shortId": q.Get("sid"), "fingerprint": q.Get("fp")}
		}
	}
	if stream["security"] == "tls" {
		stream["tlsSettings"] = map[string]any{"serverName": "127.0.0.1", "certificates": []any{map[string]any{"certificateFile": cert, "usage": "verify"}}}
	}
	return map[string]any{"protocol": protocol, "settings": settings, "streamSettings": stream}
}
func TestNativeConfigDriverRoundTrips(t *testing.T) {
	core := os.Getenv("XRAY_TEST_BINARY")
	if core == "" {
		t.Skip("XRAY_TEST_BINARY required for actual native round trips")
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "driver-roundtrip-ok") }))
	defer origin.Close()
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "reality-target") }))
	target.EnableHTTP2 = true
	target.TLS = &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, CurvePreferences: []tls.CurveID{tls.X25519}}
	target.StartTLS()
	defer target.Close()
	dir := t.TempDir()
	cert := filepath.Join(dir, "cert.pem")
	private := filepath.Join(dir, "key.pem")
	certificate := target.TLS.Certificates[0]
	if e := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0600); e != nil {
		t.Fatal(e)
	}
	key, e := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(private, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600); e != nil {
		t.Fatal(e)
	}
	registry := Builtins()
	for _, cap := range append(registry.Catalog(), Capability{ID: "vless/tcp/reality_no_flow", Protocol: "vless", Network: "tcp", Security: "reality", Public: true}) {
		t.Run(strings.ReplaceAll(cap.ID, "/", "_"), func(t *testing.T) {
			spec := Spec{Protocol: cap.Protocol, Network: cap.Network, Security: cap.Security, Port: freePort(t)}
			if spec.Network == "ws" || spec.Network == "xhttp" {
				spec.Path = "/fixture"
			}
			if spec.Network == "grpc" {
				spec.Path = "fixture"
			}
			if spec.Security == "reality" {
				spec.SNI = "example.com"
				if spec.Network == "tcp" && cap.ID != "vless/tcp/reality_no_flow" {
					spec.Flow = "xtls-rprx-vision"
				}
			}
			creds, e := NewCredentials("roundtrip", "panel", []string{"DIRECT"}, 1)
			if e != nil {
				t.Fatal(e)
			}
			server, e := registry.Build(spec, "127.0.0.1", "fixture", creds)
			if e != nil {
				t.Fatal(e)
			}
			server["listen"] = "127.0.0.1"
			server["tag"] = "fixture"
			stream := server["streamSettings"].(map[string]any)
			if spec.Security == "tls" {
				stream["tlsSettings"].(map[string]any)["certificates"] = []any{map[string]any{"certificateFile": cert, "keyFile": private}}
			}
			if spec.Security == "reality" {
				stream["realitySettings"].(map[string]any)["dest"] = strings.TrimPrefix(target.URL, "https://")
			}
			cfg := map[string]any{"log": map[string]any{"loglevel": "warning"}, "inbounds": []any{server}, "outbounds": []any{map[string]any{"protocol": "freedom", "settings": map[string]any{"finalRules": []any{map[string]any{"action": "allow", "network": "tcp", "ip": []string{"127.0.0.1/32"}}}}}}}
			coreStart(t, core, cfg, spec.Port)
			uri, e := registry.URI(spec, "127.0.0.1", "Fixture", creds, creds.Clients[0])
			if e != nil {
				t.Fatal(e)
			}
			outbound := clientFromURI(t, uri, cert)
			proxyPort := freePort(t)
			client := map[string]any{"log": map[string]any{"loglevel": "warning"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": proxyPort, "protocol": "http", "settings": map[string]any{}}}, "outbounds": []any{outbound}}
			coreStart(t, core, client, proxyPort)
			proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
			tr := &http.Transport{Proxy: http.ProxyURL(proxyURL), DisableKeepAlives: true}
			defer tr.CloseIdleConnections()
			httpClient := http.Client{Transport: tr, Timeout: 8 * time.Second}
			response, e := httpClient.Get(origin.URL)
			if e != nil {
				t.Fatal("native round trip", e)
			}
			defer response.Body.Close()
			body, e := io.ReadAll(response.Body)
			if e != nil || string(body) != "driver-roundtrip-ok" {
				t.Fatalf("round trip status=%d bytes=%d error=%v", response.StatusCode, len(body), e)
			}
		})
	}
}
