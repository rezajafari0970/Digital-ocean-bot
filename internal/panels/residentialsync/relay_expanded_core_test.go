package residentialsync

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestInstalledCoreExpandedSOCKSRelaysFailover(t *testing.T) {
	if os.Getenv("XRAY_TEST_BINARY") == "" {
		t.Skip("real Xray required")
	}
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "expanded-relay-sink") }))
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
	for i := 0; i < 6; i++ {
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
	cp.Configured = 6
	cp.Direct = false
	cp.RelayMode = true
	receiver := pathConfig(t, cp)
	// A local permitted-domain sink replaces only the HTTP probe destination.
	receiver["burstObservatory"].(map[string]any)["pingConfig"].(map[string]any)["destination"] = "http://browserleaks.com:" + pt
	address := runPathCore(t, receiver)
	success := func() bool {
		b, e := strictVLESSRequest(address, pathResID, "browserleaks.com", "browserleaks.com", sinkPort)
		return e == nil && b == "expanded-relay-sink"
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
	for i := 0; i < 5; i++ {
		stops[i]()
	}
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
		t.Fatal("five-donor outage failed to use survivor")
	}
	t.Logf("five-donor outage recovered within %s", time.Since(started).Round(time.Millisecond))
	stops[5]()
	if success() {
		t.Fatal("all-donor outage escaped direct")
	}
}
