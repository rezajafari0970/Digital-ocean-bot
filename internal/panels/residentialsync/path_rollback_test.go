package residentialsync

import (
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"os"
	"testing"
)

func TestPinnedStrictClientPathsRollback(t *testing.T) {
	raw, e := os.ReadFile("testdata/client_paths_legacy_435486a.json")
	if e != nil {
		t.Fatal(e)
	}
	golden := map[string]string{}
	if e = json.Unmarshal(raw, &golden); e != nil {
		t.Fatal(e)
	}
	for _, pool := range []bool{false, true} {
		for _, mode := range []string{"tcp", "doh"} {
			p := routePolicy{StrictAllowlist: true, LegacyClientPaths: true, StableFingerprint: true, Explicit: true, Residential: true, Direct: true, Configured: 1, PoolEnabled: pool, Proxies: []rp{{Type: "socks5", Host: "proxy.fixture.test", Port: 1080, Tag: "residential-ads-fixture", User: "test", Password: "fixture-only"}}}
			cfg := residentialperf.Balanced()
			cfg.DNSMode = mode
			p.Performance = &cfg
			cs := []clientRoute{{ID: "direct", Email: "direct@test", Class: "DIRECT", Effective: "DIRECT"}, {ID: "res", Email: "res@test", Class: "RESIDENTIAL", Effective: "RESIDENTIAL"}}
			s, e := buildSettings(baseSettings(), cs, []string{"in"}, p)
			if e != nil {
				t.Fatal(e)
			}
			key := fmt.Sprintf("%t/%s", pool, mode)
			if settingsHash(s) != golden[key] {
				t.Fatal("pinned435 mismatch", key, settingsHash(s))
			}
			p.LegacyClientPaths = false
			current, e := buildSettings(s, cs, []string{"in"}, p)
			if e != nil {
				t.Fatal(e)
			}
			cs[1].ID = "replacement"
			cs[1].Email = "replacement@test"
			p.LegacyClientPaths = true
			restored, e := buildSettings(current, cs, []string{"in"}, p)
			if e != nil || settingsHash(restored) != golden[key] {
				t.Fatal("rollback after identity turnover", key, e, settingsHash(restored))
			}
		}
	}
}
