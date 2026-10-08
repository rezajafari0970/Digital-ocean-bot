package residentialsync

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func strictVLESSConn(address, id, host string, port int, command byte) (net.Conn, error) {
	c, e := net.DialTimeout("tcp", address, time.Second)
	if e != nil {
		return nil, e
	}
	c.SetDeadline(time.Now().Add(1500 * time.Millisecond))
	uuid, e := hex.DecodeString(id)
	if e != nil {
		c.Close()
		return nil, e
	}
	h := append([]byte{0}, uuid...)
	h = append(h, 0, command, byte(port>>8), byte(port))
	ip := net.ParseIP(host)
	if ip4 := ip.To4(); ip4 != nil {
		h = append(h, 1)
		h = append(h, ip4...)
	} else if ip != nil {
		h = append(h, 3)
		h = append(h, ip.To16()...)
	} else {
		h = append(h, 2, byte(len(host)))
		h = append(h, host...)
	}
	if _, e = c.Write(h); e != nil {
		c.Close()
		return nil, e
	}
	return c, nil
}
func strictVLESSRequest(address, id, target, host string, port int) (string, error) {
	c, e := strictVLESSConn(address, id, target, port, 1)
	if e != nil {
		return "", e
	}
	defer c.Close()
	fmt.Fprintf(c, "GET /traffic HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", host)
	var h [2]byte
	if _, e = io.ReadFull(c, h[:]); e != nil {
		return "", e
	}
	if _, e = io.CopyN(io.Discard, c, int64(h[1])); e != nil {
		return "", e
	}
	response, e := http.ReadResponse(bufio.NewReader(c), nil)
	if e != nil {
		return "", e
	}
	defer response.Body.Close()
	b, e := io.ReadAll(response.Body)
	return string(b), e
}
func strictVLESSUDP(address, id, target string, port int) (string, error) {
	c, e := strictVLESSConn(address, id, target, port, 2)
	if e != nil {
		return "", e
	}
	defer c.Close()
	body := []byte("allowlist-udp")
	if _, e = c.Write(append([]byte{0, byte(len(body))}, body...)); e != nil {
		return "", e
	}
	var h [2]byte
	if _, e = io.ReadFull(c, h[:]); e != nil {
		return "", e
	}
	if _, e = io.CopyN(io.Discard, c, int64(h[1])); e != nil {
		return "", e
	}
	if _, e = io.ReadFull(c, h[:]); e != nil {
		return "", e
	}
	b := make([]byte, binary.BigEndian.Uint16(h[:]))
	_, e = io.ReadFull(c, b)
	return string(b), e
}

func TestInstalledCoreStrictAllowlistBothDirectionsAndOutage(t *testing.T) {
	binaryPath := os.Getenv("XRAY_TEST_BINARY")
	if binaryPath == "" {
		t.Skip("installed Xray required")
	}
	const directID = "00000000000040008000000000000001"
	const resID = "00000000000040008000000000000002"
	for _, pool := range []bool{false, true} {
		t.Run(fmt.Sprintf("pool=%t", pool), func(t *testing.T) {
			upstream := newPoolFixture(t, "residential", 0, nil)
			var directHits atomic.Int64
			sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { directHits.Add(1); fmt.Fprint(w, "direct") }))
			defer sink.Close()
			_, pt, _ := net.SplitHostPort(sink.Listener.Addr().String())
			sinkPort, _ := strconv.Atoi(pt)
			udp, e := net.ListenPacket("udp4", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			defer udp.Close()
			var udpHits atomic.Int64
			go func() {
				b := make([]byte, 100)
				for {
					n, a, e := udp.ReadFrom(b)
					if e != nil {
						return
					}
					udpHits.Add(1)
					udp.WriteTo(b[:n], a)
				}
			}()
			udpPort := udp.LocalAddr().(*net.UDPAddr).Port
			p := routePolicy{StrictAllowlist: true, PoolEnabled: pool, Explicit: true, Residential: true, Direct: true, Configured: 1,
				Proxies: []rp{{Type: "socks5", Host: "127.0.0.1", Port: upstream.listener.Addr().(*net.TCPAddr).Port, Tag: "residential-ads-fixture"}}}
			clients := []clientRoute{{ID: directID, Email: "direct@test", Class: "DIRECT", Effective: "DIRECT"}, {ID: resID, Email: "res@test", Class: "RESIDENTIAL", Effective: "RESIDENTIAL"}}
			setting, e := buildSettings(map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"rules": []any{}}}, clients, []string{"in"}, p)
			if e != nil {
				t.Fatal(e)
			}
			if pool {
				// Local fixture observer; no external traffic in this test.
				ping := setting["burstObservatory"].(map[string]any)["pingConfig"].(map[string]any)
				ping["destination"] = "http://connectivitycheck.gstatic.com/probe"
				ping["interval"] = "1s"
				ping["timeout"] = "500ms"
			}
			l, e := net.Listen("tcp4", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			port := l.Addr().(*net.TCPAddr).Port
			l.Close()
			setting["inbounds"] = []any{map[string]any{"tag": "in", "listen": "127.0.0.1", "port": port, "protocol": "vless",
				"settings": map[string]any{"decryption": "none", "clients": []any{
					map[string]any{"id": "00000000-0000-4000-8000-000000000001", "email": "direct@test"},
					map[string]any{"id": "00000000-0000-4000-8000-000000000002", "email": "res@test"}}},
				"sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": false}}}
			// v26.9.9 Freedom denies loopback by default. Only this isolated
			// local-sink fixture allows it; production egress guards stay intact.
			for _, value := range setting["outbounds"].([]any) {
				out := value.(map[string]any)
				if out["protocol"] == "freedom" {
					out["settings"] = map[string]any{"finalRules": []any{map[string]any{"action": "allow"}}}
				}
			}
			setting["log"] = map[string]any{"loglevel": "debug"}
			raw, _ := json.Marshal(setting)
			file := filepath.Join(t.TempDir(), "core.json")
			if e = os.WriteFile(file, raw, 0600); e != nil {
				t.Fatal(e)
			}
			if out, e := exec.Command(binaryPath, "run", "-test", "-config", file).CombinedOutput(); e != nil {
				t.Fatalf("config %v %s", e, out)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cmd := exec.CommandContext(ctx, binaryPath, "run", "-config", file)
			var coreLog bytes.Buffer
			cmd.Stdout = &coreLog
			cmd.Stderr = &coreLog
			if e = cmd.Start(); e != nil {
				cancel()
				t.Fatal(e)
			}
			defer func() {
				cancel()
				cmd.Wait()
				if t.Failed() {
					text := coreLog.String()
					if len(text) > 16000 {
						text = text[len(text)-16000:]
					}
					t.Log(text)
				}
			}()
			address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
			poolWait(t, "listener", func() bool {
				c, e := net.DialTimeout("tcp", address, 50*time.Millisecond)
				if e == nil {
					c.Close()
				}
				return e == nil
			})
			if pool {
				poolWait(t, "native pool healthy", func() bool { return upstream.checks.Load() > 0 })
			}
			for _, domain := range []string{"adservice.google.com", "browserleaks.com", "tls.browserleaks.com", "www.gstatic.com", "connectivitycheck.gstatic.com"} {
				for _, port := range []int{80, 443} {
					if body, e := strictVLESSRequest(address, resID, domain, domain, port); e != nil || body != "residential" {
						t.Fatalf("allowed %s:%d body=%q err=%v", domain, port, body, e)
					}
				}
			}
			// Allowed sniffed name must replace the arbitrary requested IP before the proxy sees it.
			if body, e := strictVLESSRequest(address, resID, "127.0.0.1", "adservice.google.com", sinkPort); e != nil || body != "residential" {
				t.Fatal("sniff override", body, e)
			}
			if upstream.targetType.Load() != 3 || directHits.Load() != 0 {
				t.Fatal("arbitrary IP retained after sniff")
			}
			beforeTCP := upstream.tcpHits.Load()
			beforeUDP := upstream.udpHits.Load()
			for _, target := range []struct {
				host string
				port int
			}{
				{"127.0.0.1", sinkPort}, {"www.google.com", 443}, {"www.gstatic.com.evil.test", 443},
				{"evil.www.gstatic.com", 443}, {"www.gstatic.com", 8443}, {"8.8.8.8", 53}, {"adservice.google.com", 53},
				{"browserleaks.com.evil.test", 443},
			} {
				if body, e := strictVLESSRequest(address, resID, target.host, target.host, target.port); e == nil {
					t.Fatalf("denied got response %s %q", target.host, body)
				}
			}
			if body, e := strictVLESSUDP(address, resID, "adservice.google.com", 80); e != nil || body != "residential" {
				t.Fatal("allowed UDP roundtrip", body, e)
			}
			for _, target := range []struct {
				host string
				port int
			}{{"127.0.0.1", udpPort}, {"www.google.com", 443}, {"www.gstatic.com", 443}, {"8.8.8.8", 53}} {
				if body, e := strictVLESSUDP(address, resID, target.host, target.port); e == nil {
					t.Fatalf("denied UDP got response %s %q", target.host, body)
				}
			}
			if upstream.tcpHits.Load() != beforeTCP || upstream.udpHits.Load() != beforeUDP+1 || directHits.Load() != 0 || udpHits.Load() != 0 {
				t.Fatal("forbidden traffic reached an egress")
			}
			if body, e := strictVLESSRequest(address, directID, "127.0.0.1", "127.0.0.1", sinkPort); e != nil || body != "direct" {
				t.Fatal("DIRECT positive control", body, e)
			}
			if body, e := strictVLESSUDP(address, directID, "127.0.0.1", udpPort); e != nil || body != "allowlist-udp" {
				t.Fatal("DIRECT UDP control", body, e)
			}
			upstream.down.Store(true)
			for _, host := range []string{"adservice.google.com", "www.gstatic.com"} {
				if _, e := strictVLESSRequest(address, resID, host, host, 80); e == nil {
					t.Fatal("failed proxy succeeded")
				}
			}
			if directHits.Load() != 1 || udpHits.Load() != 1 {
				t.Fatal("outage leaked direct")
			}
			upstream.down.Store(false)
			if pool {
				poolWait(t, "pool recovery", func() bool {
					body, e := strictVLESSRequest(address, resID, "adservice.google.com", "adservice.google.com", 80)
					return e == nil && body == "residential"
				})
			}
			if body, e := strictVLESSRequest(address, resID, "browserleaks.com", "browserleaks.com", 80); e != nil || body != "residential" {
				t.Fatal("recovery", body, e)
			}
		})
	}
}
