package residentialsync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// This test runs the installed core, not the in-memory route matcher. A local
// sink proves category-only routing and detects Ads escaping a failed proxy.
func TestInstalledCoreAdsFailClosedAndOtherTCPUDPDirect(t *testing.T) {
	binaryPath := os.Getenv("XRAY_TEST_BINARY")
	if binaryPath == "" {
		t.Skip("set XRAY_TEST_BINARY for installed-core fault acceptance")
	}
	var hits atomic.Int64
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); fmt.Fprint(w, "sink") }))
	defer sink.Close()
	_, portText, _ := net.SplitHostPort(sink.Listener.Addr().String())
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	udpPort := udp.LocalAddr().(*net.UDPAddr).Port
	for _, kind := range []string{"direct-control", "absent", "upstream-refused"} {
		t.Run(kind, func(t *testing.T) {
			before := hits.Load()
			p := routePolicy{Harden: true, AdsOnly: true, Residential: true}
			if kind == "direct-control" {
				p.Residential = false
				p.Direct = true
			}
			if kind == "upstream-refused" {
				p.Configured = 1
				p.Proxies = []rp{{Type: "socks5", Host: "127.0.0.1", Port: 1, Tag: "residential-ads-fault"}}
			}
			setting, err := buildSettings(map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"rules": []any{}}}, nil, []string{"actual-inbound-tag", "actual-vless-tag"}, p)
			if err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			proxyPort := listener.Addr().(*net.TCPAddr).Port
			listener.Close()
			vlessListener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			vlessPort := vlessListener.Addr().(*net.TCPAddr).Port
			vlessListener.Close()
			vlessAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(vlessPort))
			// Resolve fixture domains using test-only static hosts, never public DNS.
			for _, v := range setting["outbounds"].([]any) {
				o := v.(map[string]any)
				if o["protocol"] == "freedom" {
					// Permit test-only loopback sinks under the target core private-IP default.
					o["settings"] = map[string]any{"domainStrategy": "UseIPv4", "finalRules": []any{map[string]any{"action": "allow"}}}
				}
			}
			setting["dns"].(map[string]any)["hosts"] = map[string]any{"adservice.google.com": "127.0.0.1", "browserleaks.com": "127.0.0.1"}
			setting["log"] = map[string]any{"loglevel": "debug"}
			setting["inbounds"] = []any{map[string]any{"tag": "actual-inbound-tag", "listen": "127.0.0.1", "port": proxyPort, "protocol": "socks", "settings": map[string]any{"auth": "noauth", "udp": true}, "sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "metadataOnly": false}}}
			setting["inbounds"] = append(setting["inbounds"].([]any), map[string]any{"tag": "actual-vless-tag", "listen": "127.0.0.1", "port": vlessPort, "protocol": "vless", "settings": map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": "00000000-0000-4000-8000-000000000002", "email": "fixture@test"}}}})

			file := filepath.Join(t.TempDir(), "core.json")
			raw, _ := json.Marshal(setting)
			if err = os.WriteFile(file, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(binaryPath, "run", "-test", "-config", file).CombinedOutput(); err != nil {
				t.Fatalf("core rejects config: %s %v", out, err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd := exec.CommandContext(ctx, binaryPath, "run", "-config", file)
			var coreLog bytes.Buffer
			cmd.Stdout = &coreLog
			cmd.Stderr = &coreLog
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				cancel()
				cmd.Wait()
				if t.Failed() {
					text := coreLog.String()
					if len(text) > 10000 {
						text = text[len(text)-10000:]
					}
					t.Log(text)
				}
			}()
			address := net.JoinHostPort("127.0.0.1", strconv.Itoa(proxyPort))
			ready := false
			for i := 0; i < 50; i++ {
				c, e := net.DialTimeout("tcp", address, 50*time.Millisecond)
				if e == nil {
					c.Close()
					ready = true
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if !ready {
				t.Fatal("core listener unavailable")
			}
			for _, target := range []string{sink.URL, "http://adservice.google.com:" + portText, "http://browserleaks.com:" + portText} {
				_, e := exec.Command("curl", "--silent", "--max-time", "2", "--proxy", "socks5h://"+address, "--noproxy", "", target).CombinedOutput()
				blocked := kind != "direct-control" && (strings.Contains(target, "adservice.google.com") || strings.Contains(target, "browserleaks.com"))
				if blocked && e == nil {
					t.Fatal("Ads escaped failed residential proxy")
				}
				if !blocked && e != nil {
					t.Fatal("non-Ad TCP did not remain direct", target, e)
				}
			}
			// Exercise the deployed VLESS UDP transport. Newer SOCKS inbounds
			// resolve UDP domain destinations before dispatch; that transport
			// cannot stand in for VLESS domain-routing acceptance.
			receive := func(target string) (bool, error) {
				result := make(chan error, 1)
				go func() {
					_, e := strictVLESSUDP(vlessAddress, "00000000000040008000000000000002", target, udpPort)
					result <- e
				}()
				udp.SetReadDeadline(time.Now().Add(1200 * time.Millisecond))
				buf := make([]byte, 100)
				n, addr, e := udp.ReadFrom(buf)
				if e == nil {
					udp.WriteTo(buf[:n], addr)
				}
				clientErr := <-result
				return e == nil && n > 0, clientErr
			}
			if reached, e := receive("127.0.0.1"); !reached || e != nil {
				t.Fatal("non-ad UDP direct failed", reached, e)
			}
			reached, e := receive("adservice.google.com")
			if kind == "direct-control" {
				if !reached || e != nil {
					t.Fatal("direct control Ads UDP failed", reached, e)
				}
			} else if reached || e == nil {
				t.Fatal("Ads UDP escaped failed residential upstream")
			}
			want := int64(1)
			if kind == "direct-control" {
				want = 3
			}
			if hits.Load() != before+want {
				t.Fatal("unexpected direct sink traffic", hits.Load()-before, want)
			}

		})
	}
}
