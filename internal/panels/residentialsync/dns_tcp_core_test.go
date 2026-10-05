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
	"sync/atomic"
	"testing"
	"time"
)

func TestInstalledCoreResidentialUDPDNSIsBlockedBeforeResolver(t *testing.T) {
	binaryPath := os.Getenv("XRAY_TEST_BINARY")
	if binaryPath == "" {
		t.Skip("installed Xray required")
	}
	var directHits, proxyTCP, proxyUDP atomic.Int64
	var available atomic.Bool
	available.Store(true)
	sink, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	go func() {
		for {
			c, e := sink.Accept()
			if e != nil {
				return
			}
			directHits.Add(1)
			c.Close()
		}
	}()
	upstream, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()
	go func() {
		for {
			conn, e := upstream.Accept()
			if e != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				c.SetDeadline(time.Now().Add(8 * time.Second))
				head := make([]byte, 2)
				if _, e := io.ReadFull(c, head); e != nil {
					return
				}
				methods := make([]byte, int(head[1]))
				if _, e := io.ReadFull(c, methods); e != nil {
					return
				}
				c.Write([]byte{5, 0})
				request := make([]byte, 4)
				if _, e := io.ReadFull(c, request); e != nil {
					return
				}
				if request[1] != 1 {
					proxyUDP.Add(1)
					c.Write([]byte{5, 7, 0, 1, 0, 0, 0, 0, 0, 0})
					return
				}
				n := 0
				switch request[3] {
				case 1:
					n = 4
				case 4:
					n = 16
				case 3:
					b := make([]byte, 1)
					if _, e := io.ReadFull(c, b); e != nil {
						return
					}
					n = int(b[0])
				default:
					return
				}
				target := make([]byte, n+2)
				if _, e := io.ReadFull(c, target); e != nil {
					return
				}
				if !available.Load() {
					c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
					return
				}
				proxyTCP.Add(1)
				c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
				for {
					size := make([]byte, 2)
					if _, e := io.ReadFull(c, size); e != nil {
						return
					}
					q := make([]byte, int(binary.BigEndian.Uint16(size)))
					if _, e := io.ReadFull(c, q); e != nil || len(q) < 16 || !available.Load() {
						return
					}
					answer := append([]byte(nil), q...)
					answer[2] = 0x81
					answer[3] = 0x80
					answer[6] = 0
					answer[7] = 0
					if binary.BigEndian.Uint16(q[len(q)-4:]) == 1 {
						answer[7] = 1
						answer = append(answer, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 5, 0, 4, 198, 51, 100, 7)
					}
					binary.BigEndian.PutUint16(size, uint16(len(answer)))
					if _, e := c.Write(append(size, answer...)); e != nil {
						return
					}
				}
			}(conn)
		}
	}()
	policy := routePolicy{Harden: true, AdsOnly: true, Residential: true, Configured: 1, Proxies: []rp{{Type: "socks5", Host: "127.0.0.1", Port: upstream.Addr().(*net.TCPAddr).Port, Tag: "residential-ads-dns-test"}}}
	setting, err := buildSettings(baseSettings(), nil, []string{"dns-test-inbound"}, policy)
	if err != nil {
		t.Fatal(err)
	}
	// Observe any unexpected DNS connection caused by the blocked datagram.
	// Use its loopback address instead of sending fault tests to public DNS.
	dns := setting["dns"].(map[string]any)
	dns["servers"] = []string{"tcp://" + sink.Addr().String()}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	setting["inbounds"] = []any{map[string]any{"listen": "127.0.0.1", "port": port, "tag": "dns-test-inbound", "protocol": "socks", "settings": map[string]any{"auth": "noauth", "udp": true}}}
	// baseSettings carries a test API rule but no API listener is needed here.
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
	query := func(name string, typ byte) ([]byte, error) {
		control, e := net.DialTimeout("tcp", address, time.Second)
		if e != nil {
			return nil, e
		}
		defer control.Close()
		control.SetDeadline(time.Now().Add(3 * time.Second))
		control.Write([]byte{5, 1, 0})
		buf := make([]byte, 2)
		if _, e = io.ReadFull(control, buf); e != nil {
			return nil, e
		}
		control.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0})
		bound := make([]byte, 10)
		if _, e = io.ReadFull(control, bound); e != nil {
			return nil, e
		}
		udp, e := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(binary.BigEndian.Uint16(bound[8:]))})
		if e != nil {
			return nil, e
		}
		defer udp.Close()
		q := []byte{0x42, 0x71, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, byte(len(name))}
		q = append(q, []byte(name)...)
		q = append(q, 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 0, 0, typ, 0, 1)
		// UDP DNS must be denied before any resolver or upstream connection.
		packet := append([]byte{0, 0, 0, 1, 9, 9, 9, 9, 0, 53}, q...)
		udp.SetDeadline(time.Now().Add(2 * time.Second))
		udp.Write(packet)
		response := make([]byte, 4096)
		n, e := udp.Read(response)
		if e != nil {
			return nil, e
		}
		return response[:n], nil
	}
	for _, typ := range []byte{1, 65} {
		if _, e := query("probe", typ); e == nil {
			t.Fatalf("residential UDP DNS type %d was allowed", typ)
		}
	}
	if proxyTCP.Load() != 0 || proxyUDP.Load() != 0 || directHits.Load() != 0 {
		t.Fatal("blocked UDP DNS generated upstream traffic", proxyTCP.Load(), proxyUDP.Load(), directHits.Load())
	}
}
