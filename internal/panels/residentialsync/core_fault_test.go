package residentialsync

import (
	"context"
	"encoding/binary"
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

// This test runs the installed core, not the in-memory route matcher. A local
// sink detects any actual packet that incorrectly escapes a failed proxy.
func TestInstalledCoreNeverFallsBackOnOpaqueTraffic(t *testing.T) {
	binaryPath := os.Getenv("XRAY_TEST_BINARY")
	if binaryPath == "" {
		t.Skip("set XRAY_TEST_BINARY for installed-core fault acceptance")
	}
	var hits atomic.Int64
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); fmt.Fprint(w, "sink") }))
	defer sink.Close()
	_, portText, _ := net.SplitHostPort(sink.Listener.Addr().String())
	sinkPort, _ := strconv.Atoi(portText)
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
			setting, err := buildSettings(map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"rules": []any{}}}, nil, []string{"actual-inbound-tag"}, p)
			if err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			proxyPort := listener.Addr().(*net.TCPAddr).Port
			listener.Close()
			setting["log"] = map[string]any{"loglevel": "none"}
			setting["inbounds"] = []any{map[string]any{"tag": "actual-inbound-tag", "listen": "127.0.0.1", "port": proxyPort, "protocol": "socks", "settings": map[string]any{"auth": "noauth", "udp": true}, "sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "metadataOnly": false}}}
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
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { cancel(); cmd.Wait() }()
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
			for _, target := range []string{sink.URL, "http://adservice.google.com:" + portText} {
				// The direct control is an IP request; protected tests also cover ads.
				if kind == "direct-control" && target != sink.URL {
					continue
				}
				_, e := exec.Command("curl", "--silent", "--max-time", "2", "--proxy", "socks5h://"+address, "--noproxy", "", target).CombinedOutput()
				if kind == "direct-control" && e != nil {
					t.Fatal("direct control failed", e)
				}
				if kind != "direct-control" && e == nil {
					t.Fatal("protected TCP request unexpectedly succeeded")
				}
			}
			// Actual SOCKS UDP relay; an opaque datagram must not reach the local sink.
			control, e := net.DialTimeout("tcp", address, time.Second)
			if e != nil {
				t.Fatal(e)
			}
			defer control.Close()
			control.SetDeadline(time.Now().Add(3 * time.Second))
			control.Write([]byte{5, 1, 0})
			reply := make([]byte, 2)
			if _, e = io.ReadFull(control, reply); e != nil {
				t.Fatal(e)
			}
			control.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0})
			bound := make([]byte, 10)
			if _, e = io.ReadFull(control, bound); e != nil || bound[1] != 0 {
				t.Fatal("UDP associate", e)
			}
			remote := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(binary.BigEndian.Uint16(bound[8:10]))}
			conn, e := net.DialUDP("udp4", nil, remote)
			if e != nil {
				t.Fatal(e)
			}
			defer conn.Close()
			packet := []byte{0, 0, 0, 1, 127, 0, 0, 1, byte(udpPort >> 8), byte(udpPort)}
			packet = append(packet, []byte("opaque-no-hostname")...)
			conn.Write(packet)
			udp.SetReadDeadline(time.Now().Add(350 * time.Millisecond))
			buf := make([]byte, 100)
			n, _, e := udp.ReadFrom(buf)
			if kind == "direct-control" {
				if e != nil || n == 0 {
					t.Fatal("UDP direct control failed", e)
				}
			} else if e == nil {
				t.Fatal("opaque UDP leaked")
			}
			if kind == "direct-control" {
				if hits.Load() != before+1 {
					t.Fatal("TCP control did not reach sink")
				}
			} else if hits.Load() != before {
				t.Fatal("TCP leaked to direct sink", sinkPort)
			}
		})
	}
}
