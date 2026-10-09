package residentialsync

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"io"
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

const pathResID = "00000000000040008000000000000002"
const pathDirectID = "00000000000040008000000000000001"

func pathPolicy(pool bool) routePolicy {
	return routePolicy{StrictAllowlist: true, PoolEnabled: pool, Residential: true, Direct: true, Configured: 1}
}
func pathConfig(t *testing.T, p routePolicy) map[string]any {
	t.Helper()
	s, e := buildSettings(map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"domainStrategy": "AsIs", "rules": []any{}}}, []clientRoute{{ID: pathDirectID, Email: "direct@test", Class: "DIRECT", Effective: "DIRECT"}, {ID: pathResID, Email: "res@test", Class: "RESIDENTIAL", Effective: "RESIDENTIAL"}}, []string{"in"}, p)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func runPathCore(t *testing.T, s map[string]any, certs ...[]byte) string {
	t.Helper()
	binaryPath := os.Getenv("XRAY_TEST_BINARY")
	if binaryPath == "" {
		t.Skip("real Xray required")
	}
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	address := l.Addr().String()
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	s["inbounds"] = []any{map[string]any{"tag": "in", "listen": "127.0.0.1", "port": port, "protocol": "vless", "settings": map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": "00000000-0000-4000-8000-000000000001", "email": "direct@test"}, map[string]any{"id": "00000000-0000-4000-8000-000000000002", "email": "res@test"}}}, "sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": false}}}
	s["log"] = map[string]any{"loglevel": "warning"}
	dir := t.TempDir()
	raw, _ := json.Marshal(s)
	file := filepath.Join(dir, "core.json")
	os.WriteFile(file, raw, 0600)
	env := os.Environ()
	if len(certs) > 0 {
		var all []byte
		for _, der := range certs {
			all = append(all, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
		}
		ca := filepath.Join(dir, "ca.pem")
		os.WriteFile(ca, all, 0600)
		env = append(env, "SSL_CERT_FILE="+ca)
	}
	check := exec.Command(binaryPath, "run", "-test", "-config", file)
	check.Env = env
	if out, e := check.CombinedOutput(); e != nil {
		t.Fatalf("core config: %v %s", e, out)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binaryPath, "run", "-config", file)
	cmd.Env = env
	log, _ := os.Create(filepath.Join(dir, "core.log"))
	cmd.Stdout = log
	cmd.Stderr = log
	if e := cmd.Start(); e != nil {
		cancel()
		t.Fatal(e)
	}
	t.Cleanup(func() {
		cancel()
		cmd.Wait()
		log.Close()
		if t.Failed() {
			b, _ := os.ReadFile(filepath.Join(dir, "core.log"))
			t.Log(string(b))
		}
	})
	poolWait(t, "core listener", func() bool {
		c, e := net.DialTimeout("tcp", address, 20*time.Millisecond)
		if e == nil {
			c.Close()
		}
		return e == nil
	})
	return address
}

type pathVLESSStream struct {
	net.Conn
	header bool
}

func (c *pathVLESSStream) Read(p []byte) (int, error) {
	if !c.header {
		var h [2]byte
		if _, e := io.ReadFull(c.Conn, h[:]); e != nil {
			return 0, e
		}
		if _, e := io.CopyN(io.Discard, c.Conn, int64(h[1])); e != nil {
			return 0, e
		}
		c.header = true
	}
	return c.Conn.Read(p)
}
func pathFetch(address, id, target, host string, port int, secure bool) (string, error) {
	c, e := strictVLESSConn(address, id, target, port, 1)
	if e != nil {
		return "", e
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	var stream net.Conn = &pathVLESSStream{Conn: c}
	if secure {
		stream = tls.Client(stream, &tls.Config{ServerName: host, InsecureSkipVerify: true})
	}
	if _, e = fmt.Fprintf(stream, "GET /payload HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", host); e != nil {
		return "", e
	}
	res, e := http.ReadResponse(bufio.NewReader(stream), nil)
	if e != nil {
		return "", e
	}
	defer res.Body.Close()
	b, e := io.ReadAll(res.Body)
	return string(b), e
}

// A real SOCKS tunnel to an isolated HTTP/TLS server, not a synthetic route answer.
func pathTunnel(t *testing.T, target string) (string, *atomic.Int64, *atomic.Bool, *atomic.Int64) {
	t.Helper()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { l.Close() })
	hits := &atomic.Int64{}
	down := &atomic.Bool{}
	domainTargets := &atomic.Int64{}
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				c.SetDeadline(time.Now().Add(5 * time.Second))
				var h [2]byte
				if _, e := io.ReadFull(c, h[:]); e != nil {
					return
				}
				methods := make([]byte, int(h[1]))
				if _, e := io.ReadFull(c, methods); e != nil {
					return
				}
				c.Write([]byte{5, 0})
				var req [4]byte
				if _, e := io.ReadFull(c, req[:]); e != nil {
					return
				}
				n := 0
				switch req[3] {
				case 1:
					n = 4
				case 4:
					n = 16
				case 3:
					var one [1]byte
					if _, e := io.ReadFull(c, one[:]); e != nil {
						return
					}
					n = int(one[0])
					domainTargets.Add(1)
				default:
					return
				}
				addr := make([]byte, n+2)
				if _, e := io.ReadFull(c, addr); e != nil {
					return
				}
				if down.Load() {
					c.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
					return
				}
				up, e := net.DialTimeout("tcp", target, time.Second)
				if e != nil {
					return
				}
				defer up.Close()
				hits.Add(1)
				c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
				go io.Copy(up, c)
				io.Copy(c, up)
			}(c)
		}
	}()
	return l.Addr().String(), hits, down, domainTargets
}
func TestInstalledCoreDirectProbeAndRealTLSResidentialIsolation(t *testing.T) {
	if os.Getenv("XRAY_TEST_BINARY") == "" {
		t.Skip("real Xray required")
	}
	for _, pool := range []bool{false, true} {
		for _, secure := range []bool{false, true} {
			t.Run(fmt.Sprintf("pool=%v/TLS=%v", pool, secure), func(t *testing.T) {
				var directHits atomic.Int64
				makeSink := func(body string) *httptest.Server {
					h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/payload" && body == "direct" {
							directHits.Add(1)
						}
						fmt.Fprint(w, body)
					})
					if secure {
						return httptest.NewTLSServer(h)
					}
					return httptest.NewServer(h)
				}
				direct := makeSink("direct")
				defer direct.Close()
				res := makeSink("residential")
				defer res.Close()
				up, hits, down, domains := pathTunnel(t, res.Listener.Addr().String())
				_, portstr, _ := net.SplitHostPort(up)
				port, _ := strconv.Atoi(portstr)
				p := pathPolicy(pool)
				p.Proxies = []rp{{Type: "socks5", Host: "127.0.0.1", Port: port, Tag: "residential-ads-fixture"}}
				s := pathConfig(t, p)
				for _, v := range s["outbounds"].([]any) {
					o := v.(map[string]any)
					if o["protocol"] == "freedom" {
						o["settings"] = map[string]any{"redirect": direct.Listener.Addr().String(), "finalRules": []any{map[string]any{"action": "allow"}}}
					}
				}
				if pool {
					ping := s["burstObservatory"].(map[string]any)["pingConfig"].(map[string]any)
					ping["destination"] = res.URL + "/probe"
					ping["interval"] = "1s"
					ping["timeout"] = "500ms"
				}
				var certs [][]byte
				if secure {
					certs = append(certs, res.TLS.Certificates[0].Certificate[0])
				}
				address := runPathCore(t, s, certs...)
				if pool {
					poolWait(t, "pool healthy", func() bool {
						b, e := pathFetch(address, pathResID, "adservice.google.com", "adservice.google.com", 443, secure)
						return e == nil && b == "residential"
					})
				}
				for _, host := range []string{"www.gstatic.com", "connectivitycheck.gstatic.com", "www.google.com", "cloudflare-dns.com"} {
					ports := []int{443}
					if host != "cloudflare-dns.com" {
						ports = append(ports, 80)
					}
					for _, port := range ports {
						body, e := pathFetch(address, pathResID, "192.0.2.99", host, port, secure)
						if e != nil || body != "direct" {
							t.Fatalf("direct %s:%d: %q %v", host, port, body, e)
						}
					}
				}
				before := directHits.Load()
				for _, host := range []string{"adservice.google.com", "browserleaks.com", "tls.browserleaks.com"} {
					body, e := pathFetch(address, pathResID, "192.0.2.99", host, 443, secure)
					if e != nil || body != "residential" {
						t.Fatalf("protected %s: %q %v", host, body, e)
					}
				}
				if domains.Load() == 0 || hits.Load() == 0 || directHits.Load() != before {
					t.Fatal("protected TLS lacked domain override or leaked direct")
				}
				for _, x := range []struct {
					host string
					port int
				}{{"example.com", 443}, {"www.gstatic.com.evil.test", 443}, {"www.google.com", 8443}, {"cloudflare-dns.com", 80}, {"dns.google", 443}, {"1.1.1.1", 443}} {
					if _, e := pathFetch(address, pathResID, x.host, x.host, x.port, secure); e == nil {
						t.Fatal("forbidden response", x)
					}
				}
				if directHits.Load() != before {
					t.Fatal("forbidden direct attempt")
				}
				down.Store(true)
				for _, host := range []string{"browserleaks.com", "adservice.google.com"} {
					if _, e := pathFetch(address, pathResID, host, host, 443, secure); e == nil {
						t.Fatal("outage unexpectedly succeeded")
					}
				}
				if directHits.Load() != before {
					t.Fatal("residential outage direct leak")
				}
				for _, host := range []string{"www.gstatic.com", "www.google.com", "cloudflare-dns.com"} {
					body, e := pathFetch(address, pathResID, host, host, 443, secure)
					if e != nil || body != "direct" {
						t.Fatal("infrastructure depends on residential", host, body, e)
					}
				}
				if _, e := pathFetch(address, "00000000000040008000000000000009", "www.gstatic.com", "www.gstatic.com", 443, secure); e == nil {
					t.Fatal("unauthenticated request succeeded")
				}
				body, e := pathFetch(address, pathDirectID, "example.com", "example.com", 443, secure)
				if e != nil || body != "direct" {
					t.Fatal("DIRECT identity changed", body, e)
				}
			})
		}
	}
}
func pathDNSQuery(address, network, ip, name string, typ uint16) ([]byte, error) {
	cmd := byte(1)
	if network == "udp" {
		cmd = 2
	}
	c, e := strictVLESSConn(address, pathResID, ip, 53, cmd)
	if e != nil {
		return nil, e
	}
	defer c.Close()
	q := []byte{0x42, 0x71, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0}
	for _, label := range strings.Split(name, ".") {
		q = append(q, byte(len(label)))
		q = append(q, label...)
	}
	q = append(q, 0, byte(typ>>8), byte(typ), 0, 1)
	c.Write(append([]byte{byte(len(q) >> 8), byte(len(q))}, q...))
	stream := &pathVLESSStream{Conn: c}
	var size [2]byte
	if _, e = io.ReadFull(stream, size[:]); e != nil {
		return nil, e
	}
	a := make([]byte, binary.BigEndian.Uint16(size[:]))
	_, e = io.ReadFull(stream, a)
	return a, e
}
func TestInstalledCoreStrictClientDNSBothModes(t *testing.T) {
	if os.Getenv("XRAY_TEST_BINARY") == "" {
		t.Skip("real Xray required")
	}
	for _, mode := range []string{"tcp", "doh"} {
		t.Run(mode, func(t *testing.T) {
			var hits atomic.Int64
			answer := func(q []byte) []byte {
				if len(q) < 17 {
					return nil
				}
				hits.Add(1)
				// Real DoH queries may carry EDNS padding/options after the
				// question; build the answer from that question, not the tail.
				end := 12
				for end < len(q) && q[end] != 0 {
					end += 1 + int(q[end])
				}
				end++
				if end+4 > len(q) {
					return nil
				}
				typ := binary.BigEndian.Uint16(q[end : end+2])
				a := append([]byte{}, q[:end+4]...)
				a[8], a[9], a[10], a[11] = 0, 0, 0, 0
				a[2] = 0x81
				a[3] = 0x80
				a[6] = 0
				a[7] = 0
				if typ == 1 {
					a[7] = 1
					a = append(a, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 198, 51, 100, 7)
				}
				return a
			}
			resolver, e := net.Listen("tcp4", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			defer resolver.Close()
			go func() {
				for {
					c, e := resolver.Accept()
					if e != nil {
						return
					}
					go func(c net.Conn) {
						defer c.Close()
						c.SetDeadline(time.Now().Add(5 * time.Second))
						for {
							var size [2]byte
							if _, e := io.ReadFull(c, size[:]); e != nil {
								return
							}
							q := make([]byte, binary.BigEndian.Uint16(size[:]))
							if _, e := io.ReadFull(c, q); e != nil {
								return
							}
							a := answer(q)
							binary.BigEndian.PutUint16(size[:], uint16(len(a)))
							c.Write(append(size[:], a...))
						}
					}(c)
				}
			}()
			doh := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q, _ := io.ReadAll(r.Body)
				if r.Method == "GET" {
					q, _ = base64.RawURLEncoding.DecodeString(r.URL.Query().Get("dns"))
				}
				w.Header().Set("Content-Type", "application/dns-message")
				w.Write(answer(q))
			}))
			doh.EnableHTTP2 = true
			doh.StartTLS()
			defer doh.Close()
			p := pathPolicy(false)
			cfg := residentialperf.Balanced()
			cfg.DNSMode = mode
			p.Performance = &cfg
			s := pathConfig(t, p)
			server := "tcp://" + resolver.Addr().String()
			_, portstr, _ := net.SplitHostPort(resolver.Addr().String())
			if mode == "doh" {
				server = doh.URL + "/dns-query"
				_, portstr, _ = net.SplitHostPort(doh.Listener.Addr().String())
			}
			s["dns"].(map[string]any)["servers"] = []string{server}
			for _, v := range s["routing"].(map[string]any)["rules"].([]any) {
				x := v.(map[string]any)
				if x["ruleTag"] == "dob-route-dns" {
					x["ip"] = []string{"127.0.0.1"}
					x["port"] = portstr
				}
			}
			for _, v := range s["outbounds"].([]any) {
				x := v.(map[string]any)
				if x["protocol"] == "freedom" {
					x["settings"] = map[string]any{"finalRules": []any{map[string]any{"action": "allow"}}}
				}
			}
			address := runPathCore(t, s, doh.TLS.Certificates[0].Certificate[0])
			for _, ip := range []string{"1.1.1.1", "8.8.8.8"} {
				for _, network := range []string{"tcp", "udp"} {
					a, e := pathDNSQuery(address, network, ip, "browserleaks.com", 1)
					if e != nil || len(a) < 12 || binary.BigEndian.Uint16(a[6:8]) != 1 {
						t.Fatalf("A %s/%s: %v %x", network, ip, e, a)
					}
					a, e = pathDNSQuery(address, network, ip, "browserleaks.com", 28)
					if e != nil || len(a) < 12 || binary.BigEndian.Uint16(a[6:8]) != 0 {
						t.Fatalf("AAAA policy: %v %x", e, a)
					}
				}
			}
			if hits.Load() != 1 {
				t.Fatal("cache/IPv4 bound", hits.Load())
			}
			for _, network := range []string{"tcp", "udp"} {
				if _, e := pathDNSQuery(address, network, "1.1.1.1", "nonip.example", 16); e == nil {
					t.Fatal("non-IP query did not drop")
				}
				if _, e := pathDNSQuery(address, network, "9.9.9.9", "browserleaks.com", 1); e == nil {
					t.Fatal("unapproved resolver accepted")
				}
			}
			if hits.Load() != 1 {
				t.Fatal("forbidden query forwarded", hits.Load())
			}
		})
	}
}
