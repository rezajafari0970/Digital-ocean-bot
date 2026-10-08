package residentialsync

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"testing"
)

func TestStrictConfiguredDoHIsInfrastructureOnly(t *testing.T) {
	c := residentialperf.Balanced()
	c.DNSMode = "doh"
	p := routePolicy{StrictAllowlist: true, Residential: true, Direct: true, Performance: &c}
	next, err := buildSettings(baseSettings(), nil, []string{"in"}, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, inbound := range []string{"in", dnsTag} {
		for _, port := range []string{"53", "443"} {
			for _, ip := range []string{"1.1.1.1", "8.8.8.8", "9.9.9.9"} {
				want := tagged(next, blockedTag)
				if inbound == dnsTag && port == "443" && ip != "9.9.9.9" {
					want = tagged(next, directTag)
				}
				if got := strictRoute(t, next, inbound, "unknown", "", ip, "tcp", port); got != want {
					t.Fatal(inbound, ip, port, got, want)
				}
			}
		}
	}
	if err := verifyRunning(context.Background(), &fakeCore{running: next}, next, nil, []string{"in"}, p); err != nil {
		t.Fatal(err)
	}
}
func TestStrictResidentialTurnoverRetainsFingerprint(t *testing.T) {
	for _, pool := range []bool{false, true} {
		p := churnPolicy()
		p.StrictAllowlist = true
		p.PoolEnabled = pool
		before, after := churnClients()
		first, e := buildSettings(baseSettings(), before, []string{"in"}, p)
		if e != nil {
			t.Fatal(e)
		}
		second, e := buildSettings(first, after, []string{"in"}, p)
		if e != nil {
			t.Fatal(e)
		}
		if settingsHash(first) != settingsHash(second) {
			t.Fatal("residential turnover reload")
		}
	}
}
