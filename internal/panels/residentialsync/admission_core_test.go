package residentialsync

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestInstalledCoreAdmissionAndExactRestore(t *testing.T) {
	binary := os.Getenv("XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("explicit native core required")
	}
	endpoint := newPoolFixture(t, "tuning", 10*time.Millisecond, nil)
	p := routePolicy{PoolEnabled: true, Harden: true, AdsOnly: true, StrictAllowlist: true, Residential: true, Configured: 12, StableFingerprint: true}
	for i := 0; i < 12; i++ {
		p.Proxies = append(p.Proxies, rp{ID: fmt.Sprintf("11111111-1111-4111-8111-%012d", i), Type: "socks5", Host: "127.0.0.1", Port: endpoint.listener.Addr().(*net.TCPAddr).Port, Tag: fmt.Sprintf("residential-ads-11111111-1111-4111-8111-%012d", i)})
	}
	before := residentialperf.Balanced()
	before.FastCount = 13
	before.FastOpen = true
	candidate := before
	candidate.ExcludedProxyIDs = []string{p.Proxies[0].ID, p.Proxies[1].ID}
	base := map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"domainStrategy": "AsIs", "rules": []any{}}}
	p.Performance = &before
	parent, e := buildSettings(base, nil, []string{"in"}, p)
	if e != nil {
		t.Fatal(e)
	}
	p.Performance = &candidate
	tuned, e := buildSettings(parent, nil, []string{"in"}, p)
	if e != nil {
		t.Fatal(e)
	}
	p.Performance = &before
	restored, e := buildSettings(tuned, nil, []string{"in"}, p)
	if e != nil {
		t.Fatal(e)
	}
	if settingsHash(parent) != settingsHash(restored) {
		t.Fatal("exact parent selection restoration differs")
	}
	for _, id := range candidate.ExcludedProxyIDs {
		b, _ := json.Marshal(tuned)
		if strings.Contains(string(b), id) {
			t.Fatal("excluded endpoint remains in native config")
		}
	}
	if len(p.Proxies) != 12 {
		t.Fatal("caller proxy slice changed")
	}
	if settingsHash(parent) == settingsHash(tuned) {
		t.Fatal("candidate selection missing from complete plan fingerprint")
	}
	for name, config := range map[string]map[string]any{"parent": parent, "candidate": tuned, "restored": restored} {
		t.Run(name, func(t *testing.T) {
			fastWant := float64(12)
			if name == "candidate" {
				fastWant = 10
			}
			// Normalize numbers through JSON as native config serialization does.
			b, _ := json.Marshal(config)
			var normalized map[string]any
			json.Unmarshal(b, &normalized)
			for _, v := range normalized["routing"].(map[string]any)["balancers"].([]any) {
				x := v.(map[string]any)
				tag := x["tag"].(string)
				want := float64(0)
				switch tag {
				case "dob-route-pool-tcp-fast", "dob-route-pool-udp-fast":
					want = fastWant
				case "dob-route-pool-tcp-all", "dob-route-pool-udp-all":
					want = fastWant
				}
				if want > 0 && x["strategy"].(map[string]any)["settings"].(map[string]any)["expected"] != want {
					t.Fatal("wrong native expected count", tag, x)
				}
			}
			config["burstObservatory"].(map[string]any)["pingConfig"].(map[string]any)["destination"] = "http://127.0.0.1:1/probe"
			config["dns"].(map[string]any)["hosts"] = map[string]any{"adservice.google.com": "127.0.0.1"}
			l, e := net.Listen("tcp4", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			port := l.Addr().(*net.TCPAddr).Port
			l.Close()
			address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
			config["log"] = map[string]any{"loglevel": "none"}
			config["inbounds"] = []any{map[string]any{"tag": "in", "listen": "127.0.0.1", "port": port, "protocol": "socks", "settings": map[string]any{"auth": "noauth", "udp": true}}}
			file := filepath.Join(t.TempDir(), "core.json")
			b, _ = json.Marshal(config)
			if e = os.WriteFile(file, b, 0600); e != nil {
				t.Fatal(e)
			}
			if out, e := exec.Command(binary, "run", "-test", "-config", file).CombinedOutput(); e != nil {
				t.Fatalf("native candidate rejected: %v %s", e, out)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cmd := exec.CommandContext(ctx, binary, "run", "-config", file)
			if e = cmd.Start(); e != nil {
				cancel()
				t.Fatal(e)
			}
			defer func() { cancel(); cmd.Wait() }()
			poolWait(t, "exact tuning TCP", func() bool { v, e := poolRequest(address); return e == nil && v == "tuning" })
			v, e := poolUDPRequest(address)
			if e != nil || v != "tuning" {
				t.Fatal("SOCKS UDP selection/guard regression", e)
			}
		})
	}
}
