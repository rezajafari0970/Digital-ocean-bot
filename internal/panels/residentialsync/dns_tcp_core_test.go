package residentialsync

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Actual installed Xray: UDP/TCP client DNS, cached A, IPv4 policy, non-A
// forwarding and resolver fallback all work while the residential proxy fails.
func TestInstalledCoreManagedDNSDirectCacheAndFallback(t *testing.T) {
	binaryPath := os.Getenv("XRAY_TEST_BINARY")
	if binaryPath == "" {
		t.Skip("installed Xray required")
	}
	var resolverHits, proxyHits atomic.Int64
	resolver, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
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
				c.SetDeadline(time.Now().Add(8 * time.Second))
				for {
					var size [2]byte
					if _, e := io.ReadFull(c, size[:]); e != nil {
						return
					}
					q := make([]byte, binary.BigEndian.Uint16(size[:]))
					if _, e := io.ReadFull(c, q); e != nil || len(q) < 17 {
						return
					}
					resolverHits.Add(1)
					a := append([]byte(nil), q...)
					a[2] = 0x81
					a[3] = 0x80
					a[6] = 0
					a[7] = 0
					typ := binary.BigEndian.Uint16(q[len(q)-4:])
					if typ == 1 {
						a[7] = 1
						a = append(a, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 198, 51, 100, 7)
					} else if typ == 65 {
						a[3] = 0x83
					} // distinguish forwarded NXDOMAIN from synthetic empty response
					binary.BigEndian.PutUint16(size[:], uint16(len(a)))
					if _, e := c.Write(append(size[:], a...)); e != nil {
						return
					}
				}
			}(c)
		}
	}()
	proxy, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	go func() {
		for {
			c, e := proxy.Accept()
			if e != nil {
				return
			}
			proxyHits.Add(1)
			c.Close()
		}
	}()
	p := routePolicy{Harden: true, AdsOnly: true, Residential: true, Configured: 1, Proxies: []rp{{Type: "socks5", Host: "127.0.0.1", Port: proxy.Addr().(*net.TCPAddr).Port, Tag: "residential-ads-failed"}}}
	setting, err := buildSettings(baseSettings(), nil, []string{"dns-test-inbound"}, p)
	if err != nil {
		t.Fatal(err)
	}
	setting["dns"].(map[string]any)["servers"] = []string{"tcp://127.0.0.1:1", "tcp://" + resolver.Addr().String()}
	for _, v := range setting["outbounds"].([]any) {
		o := v.(map[string]any)
		if o["protocol"] == "dns" {
			s := o["settings"].(map[string]any)
			s["address"] = "127.0.0.1"
			s["port"] = resolver.Addr().(*net.TCPAddr).Port
		}
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	setting["inbounds"] = []any{map[string]any{"listen": "127.0.0.1", "port": port, "tag": "dns-test-inbound", "protocol": "socks", "settings": map[string]any{"auth": "noauth", "udp": true}}}
	delete(setting, "api")
	setting["log"] = map[string]any{"loglevel": "none"}
	file := filepath.Join(t.TempDir(), "xray.json")
	raw, _ := json.Marshal(setting)
	if err = os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if out, e := exec.Command(binaryPath, "run", "-test", "-config", file).CombinedOutput(); e != nil {
		t.Fatalf("configuration: %v %s", e, out)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, "run", "-config", file)
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); cmd.Wait() }()
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for i := 0; i < 100; i++ {
		c, e := net.DialTimeout("tcp", address, 20*time.Millisecond)
		if e == nil {
			c.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	query := func(network, name string, typ uint16) ([]byte, error) {
		c, e := net.DialTimeout("tcp", address, time.Second)
		if e != nil {
			return nil, e
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		c.Write([]byte{5, 1, 0})
		var auth [2]byte
		if _, e = io.ReadFull(c, auth[:]); e != nil {
			return nil, e
		}
		command := byte(1)
		if network == "udp" {
			command = 3
		}
		target := []byte{5, command, 0, 1, 9, 9, 9, 9, 0, 53}
		if command == 3 {
			target = []byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0}
		}
		c.Write(target)
		var bound [10]byte
		if _, e = io.ReadFull(c, bound[:]); e != nil {
			return nil, e
		}
		q := []byte{0x42, 0x71, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0}
		for _, label := range strings.Split(name, ".") {
			q = append(q, byte(len(label)))
			q = append(q, []byte(label)...)
		}
		q = append(q, 0, byte(typ>>8), byte(typ), 0, 1)
		if network == "tcp" {
			framed := []byte{byte(len(q) >> 8), byte(len(q))}
			c.Write(append(framed, q...))
			var size [2]byte
			if _, e = io.ReadFull(c, size[:]); e != nil {
				return nil, e
			}
			a := make([]byte, binary.BigEndian.Uint16(size[:]))
			_, e = io.ReadFull(c, a)
			return a, e
		}
		u, e := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(binary.BigEndian.Uint16(bound[8:]))})
		if e != nil {
			return nil, e
		}
		defer u.Close()
		u.SetDeadline(time.Now().Add(5 * time.Second))
		packet := append([]byte{0, 0, 0, 1, 9, 9, 9, 9, 0, 53}, q...)
		u.Write(packet)
		response := make([]byte, 4096)
		n, e := u.Read(response)
		if e != nil {
			return nil, e
		}
		if n < 22 {
			return nil, io.ErrUnexpectedEOF
		}
		return response[10:n], nil
	}
	for _, network := range []string{"udp", "tcp"} {
		a, e := query(network, "cached.example", 1)
		if e != nil || len(a) < 12 || a[3]&15 != 0 || binary.BigEndian.Uint16(a[6:8]) == 0 {
			t.Fatalf("DNS A %s: %v %x", network, e, a)
		}
	}
	firstHits := resolverHits.Load()
	if firstHits != 1 {
		t.Fatalf("cache/fallback expected exactly one live resolver query, got %d", firstHits)
	}
	for _, network := range []string{"udp", "tcp"} {
		a, e := query(network, "nonip.example", 65)
		if e != nil || len(a) < 12 || a[3]&15 != 3 {
			t.Fatalf("DNS HTTPS not forwarded %s: %v %x", network, e, a)
		}
		a, e = query(network, "cached.example", 28)
		if e != nil || len(a) < 12 || a[3]&15 != 0 || binary.BigEndian.Uint16(a[6:8]) != 0 {
			t.Fatalf("IPv4-only AAAA %s: %v %x", network, e, a)
		}
	}
	if proxyHits.Load() != 0 {
		t.Fatal("DNS depended on residential upstream")
	}
	if resolverHits.Load() != 3 {
		t.Fatalf("unexpected DNS requests: %d", resolverHits.Load())
	}
}
