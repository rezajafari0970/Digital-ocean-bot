package residentialsync

import (
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
	"sync"
	"testing"
	"time"
)

func relayCore(t *testing.T, cfg map[string]any, address string) func() {
	t.Helper()
	binary := os.Getenv("XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("real Xray required")
	}
	dir := t.TempDir()
	raw, _ := json.Marshal(cfg)
	file := filepath.Join(dir, "config.json")
	if e := os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	if out, e := exec.Command(binary, "run", "-test", "-config", file).CombinedOutput(); e != nil {
		t.Fatalf("core rejected fixture: %v %s", e, out)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, "run", "-config", file)
	log, e := os.Create(filepath.Join(dir, "core.log"))
	if e != nil {
		t.Fatal(e)
	}
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Start(); e != nil {
		cancel()
		t.Fatal(e)
	}
	var once sync.Once
	stop := func() { once.Do(func() { cancel(); cmd.Wait(); log.Close() }) }
	t.Cleanup(func() {
		stop()
		if t.Failed() {
			b, _ := os.ReadFile(filepath.Join(dir, "core.log"))
			t.Log(string(b))
		}
	})
	poolWait(t, "relay core listener", func() bool {
		c, e := net.DialTimeout("tcp", address, 30*time.Millisecond)
		if e == nil {
			c.Close()
		}
		return e == nil
	})
	return stop
}
func relayFreePort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
func TestInstalledCoreSOCKSRelayTLSXUDPAndFailure(t *testing.T) {
	if os.Getenv("XRAY_TEST_BINARY") == "" {
		t.Skip("real Xray required")
	}
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "residential-sink") }))
	defer sink.Close()
	_, pt, _ := net.SplitHostPort(sink.Listener.Addr().String())
	sinkPort, _ := strconv.Atoi(pt)
	udp, e := net.ListenPacket("udp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer udp.Close()
	go func() {
		b := make([]byte, 8192)
		for {
			n, a, e := udp.ReadFrom(b)
			if e != nil {
				return
			}
			udp.WriteTo(b[:n], a)
		}
	}()
	leafPort := relayFreePort(t)
	leaf := map[string]any{"log": map[string]any{"loglevel": "warning"}, "dns": map[string]any{"hosts": map[string]any{"browserleaks.com": "127.0.0.1"}},
		"inbounds":  []any{map[string]any{"tag": "leaf", "listen": "127.0.0.1", "port": leafPort, "protocol": "socks", "settings": map[string]any{"auth": "noauth", "udp": true}}},
		"outbounds": []any{map[string]any{"protocol": "freedom", "settings": map[string]any{"domainStrategy": "UseIPv4", "finalRules": []any{map[string]any{"action": "allow"}}}}}}
	relayCore(t, leaf, net.JoinHostPort("127.0.0.1", strconv.Itoa(leafPort)))
	c, e := newRelayCredential()
	if e != nil {
		t.Fatal(e)
	}
	id := "00000000-0000-4000-8000-000000000011"
	p := routePolicy{StrictAllowlist: true, Harden: true, AdsOnly: true, Residential: true, Configured: 1, StableFingerprint: true,
		Proxies:    []rp{{ID: "leaf", Tag: "residential-ads-leaf", Type: "socks5", Host: "127.0.0.1", Port: leafPort}},
		RelayInlet: &relayInlet{Panel: id, Host: "127.0.0.2", Port: 8080, Credential: c, Sources: []string{"127.0.0.1/32"}}}
	donor, e := buildSettings(map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"domainStrategy": "AsIs", "rules": []any{}}}, nil, nil, p)
	if e != nil {
		t.Fatal(e)
	}
	stop := relayCore(t, donor, "127.0.0.2:8080")
	relay, e := managedSOCKS(id, "203.0.113.2", "fixture", 8080, c)
	if e != nil {
		t.Fatal(e)
	}
	relay.Host = "127.0.0.2" // fixture-only loopback
	cp := pathPolicy(false)
	cp.Proxies = []rp{relay}
	cp.Direct = false
	receiver := pathConfig(t, cp)
	address := runPathCore(t, receiver)
	body, e := strictVLESSRequest(address, pathResID, "browserleaks.com", "browserleaks.com", sinkPort)
	if e != nil || body != "residential-sink" {
		t.Fatal("SOCKS TLS TCP path", body, e)
	}
	body, e = strictVLESSUDP(address, pathResID, "browserleaks.com", udp.LocalAddr().(*net.UDPAddr).Port)
	if e != nil || body == "" {
		t.Fatal("SOCKS TLS XUDP path", body, e)
	}
	// Ordinary UDP associate is disabled. The successful UDP request above
	// therefore used the authenticated TLS TCP connection on 8080.
	if body, e = strictVLESSRequest(address, pathResID, "example.com", "example.com", sinkPort); e == nil && body == "residential-sink" {
		t.Fatal("non-category escaped")
	}
	for _, kind := range []string{"wrong-password", "wrong-certificate"} {
		t.Run(kind, func(t *testing.T) {
			bad := relay
			if kind == "wrong-password" {
				bad.Password = "incorrect-password"
			} else {
				c2, e := newRelayCredential()
				if e != nil {
					t.Fatal(e)
				}
				copy := *bad.Relay
				copy.Certificate = c2.Certificate
				bad.Relay = &copy
			}
			q := cp
			q.Proxies = []rp{bad}
			addr := runPathCore(t, pathConfig(t, q))
			body, e := strictVLESSRequest(addr, pathResID, "browserleaks.com", "browserleaks.com", sinkPort)
			if e == nil && body == "residential-sink" {
				t.Fatal("unauthorized relay succeeded")
			}
		})
	}
	stop()
	if body, e = strictVLESSRequest(address, pathResID, "browserleaks.com", "browserleaks.com", sinkPort); e == nil && body == "residential-sink" {
		t.Fatal("relay outage escaped direct")
	}
	p.RelayInlet.Sources = []string{"127.0.0.9/32"}
	denied, e := buildSettings(map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"domainStrategy": "AsIs", "rules": []any{}}}, nil, nil, p)
	if e != nil {
		t.Fatal(e)
	}
	relayCore(t, denied, "127.0.0.2:8080")
	if body, e = strictVLESSRequest(address, pathResID, "browserleaks.com", "browserleaks.com", sinkPort); e == nil && body == "residential-sink" {
		t.Fatal("foreign source IP admitted")
	}
}

func TestInstalledCoreThreeSOCKSRelaysFailover(t *testing.T) {
	if os.Getenv("XRAY_TEST_BINARY") == "" {
		t.Skip("real Xray required")
	}
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "three-relay-sink") }))
	defer sink.Close()
	_, pt, _ := net.SplitHostPort(sink.Listener.Addr().String())
	sinkPort, _ := strconv.Atoi(pt)
	leafPort := relayFreePort(t)
	leaf := map[string]any{"log": map[string]any{"loglevel": "warning"}, "dns": map[string]any{"hosts": map[string]any{"browserleaks.com": "127.0.0.1"}},
		"inbounds":  []any{map[string]any{"tag": "leaf", "listen": "127.0.0.1", "port": leafPort, "protocol": "socks", "settings": map[string]any{"auth": "noauth", "udp": true}}},
		"outbounds": []any{map[string]any{"protocol": "freedom", "settings": map[string]any{"domainStrategy": "UseIPv4", "finalRules": []any{map[string]any{"action": "allow"}}}}}}
	relayCore(t, leaf, net.JoinHostPort("127.0.0.1", strconv.Itoa(leafPort)))
	proxies := []rp{}
	stops := []func(){}
	for i := 0; i < 3; i++ {
		c, e := newRelayCredential()
		if e != nil {
			t.Fatal(e)
		}
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", i+20)
		host := fmt.Sprintf("127.0.0.%d", i+2)
		p := routePolicy{StrictAllowlist: true, Harden: true, AdsOnly: true, Residential: true, Configured: 1, StableFingerprint: true,
			Proxies:    []rp{{ID: "leaf", Tag: "residential-ads-leaf", Type: "socks5", Host: "127.0.0.1", Port: leafPort}},
			RelayInlet: &relayInlet{Panel: id, Host: host, Port: 8080, Credential: c, Sources: []string{"127.0.0.1/32"}}}
		donor, e := buildSettings(map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"domainStrategy": "AsIs", "rules": []any{}}}, nil, nil, p)
		if e != nil {
			t.Fatal(e)
		}
		stops = append(stops, relayCore(t, donor, host+":8080"))
		proxy, e := managedSOCKS(id, fmt.Sprintf("203.0.113.%d", i+2), "fixture", 8080, c)
		if e != nil {
			t.Fatal(e)
		}
		proxy.Host = host
		proxies = append(proxies, proxy)
	}
	cp := pathPolicy(true)
	cp.Proxies = proxies
	cp.Configured = 3
	cp.Direct = false
	cp.RelayMode = true
	receiver := pathConfig(t, cp)
	// A local permitted-domain sink replaces only the HTTP probe destination.
	receiver["burstObservatory"].(map[string]any)["pingConfig"].(map[string]any)["destination"] = "http://browserleaks.com:" + pt
	address := runPathCore(t, receiver)
	success := func() bool {
		b, e := strictVLESSRequest(address, pathResID, "browserleaks.com", "browserleaks.com", sinkPort)
		return e == nil && b == "three-relay-sink"
	}
	deadline := time.Now().Add(25 * time.Second)
	warm := false
	for time.Now().Before(deadline) {
		if success() {
			warm = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !warm {
		t.Fatal("relay pool never became healthy")
	}
	stops[0]()
	stops[1]()
	started := time.Now()
	survived := false
	deadline = time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		if success() {
			survived = true
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !survived {
		t.Fatal("two-donor outage failed to use survivor")
	}
	t.Logf("two-donor outage recovered within %s", time.Since(started).Round(time.Millisecond))
	stops[2]()
	if success() {
		t.Fatal("all-donor outage escaped direct")
	}
}
