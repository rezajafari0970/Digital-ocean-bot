package residentialsync

import (
	"context"
	"strings"
	"testing"
)

func TestDNSResilienceScopeRollbackAndQUICPortBoundary(t *testing.T) {
	id := "b0000000-0000-4000-8000-000000000001"
	for _, v := range []string{"", "none", "bad", id + ",bad", id, "all"} {
		t.Setenv("DOB_DNS_RESILIENCE_PANELS", v)
		if DNSResilienceEnabled(id) != (v == id || v == "all") {
			t.Fatal("invalid feature scope", v)
		}
	}
	for _, pool := range []bool{false, true} {
		for _, strict := range []bool{false, true} {
			p := routePolicy{DNSResilience: true, GoogleAdsSelective: !strict, StrictAllowlist: strict, PoolEnabled: pool, Harden: true, AdsOnly: true, Residential: true, Direct: true, Configured: 2, StableFingerprint: true,
				Proxies: []rp{{ID: "partial", Type: "socks5", Host: "127.0.0.1", Port: 1, Tag: "residential-ads-partial", UDPRequired: true, QUICVerified: true},
					{ID: "unverified", Type: "socks5", Host: "127.0.0.1", Port: 2, Tag: "residential-ads-unverified", UDPRequired: true}}}
			cs, tags := fixtureClients(t, p)
			legacy := p
			legacy.DNSResilience = false
			before, e := buildSettings(baseSettings(), cs, tags, legacy)
			if e != nil {
				t.Fatal(e)
			}
			next, e := buildSettings(before, cs, tags, p)
			if e != nil {
				t.Fatal(e)
			}
			restored, e := buildSettings(next, cs, tags, legacy)
			if e != nil {
				t.Fatal(e)
			}
			if settingsHash(before) != settingsHash(restored) {
				t.Fatal("scope rollback changed legacy plan")
			}
			for _, c := range cs {
				for _, port := range []string{"443", "8443", "80"} {
					got := strictRoute(t, next, tags[0], c.Email, "adservice.google.com", "", "udp", port)
					if c.Class == "DIRECT" {
						if got != tagged(next, directTag) {
							t.Fatal("direct identity changed")
						}
						continue
					}
					if port != "443" {
						if got != tagged(next, blockedTag) {
							t.Fatal("unmeasured port admitted", port, got)
						}
					} else if pool {
						if !poolLaneMatches(next, "udp", got) {
							t.Fatal("QUIC pool not selected", got)
						}
					} else if got != tagged(next, p.Proxies[0].Tag) {
						t.Fatal("QUIC did not select verified proxy")
					}
				}
			}
			if pool {
				for _, kind := range []string{"fast", "all"} {
					inbound := poolPrefix + "in-udp-" + kind
					for _, port := range []string{"53", "80", "8443", "65535"} {
						if strictRoute(t, next, inbound, "", "adservice.google.com", "", "udp", port) != tagged(next, blockedTag) {
							t.Fatal("loopback bypass", port)
						}
					}
				}
				for _, v := range next["routing"].(map[string]any)["balancers"].([]any) {
					b := v.(map[string]any)
					if strings.HasPrefix(b["tag"].(string), poolPrefix+"udp-") && (b["tag"] == poolPrefix+"udp-all" || b["tag"] == poolPrefix+"udp-fast") {
						ss := b["selector"].([]string)
						if len(ss) != 1 || ss[0] != tagged(next, p.Proxies[0].Tag) {
							t.Fatal("unverified UDP in pool")
						}
					}
				}
			}
			if e = verifyRunning(context.Background(), &fakeCore{running: next}, next, cs, tags, p); e != nil {
				t.Fatal("native proof matrix", e)
			}
			p.Proxies[0].QUICVerified = false
			down, e := buildSettings(next, cs, tags, p)
			if e != nil {
				t.Fatal(e)
			}
			for _, c := range cs {
				if c.Class == "RESIDENTIAL" && strictRoute(t, down, tags[0], c.Email, "adservice.google.com", "", "udp", "443") != tagged(down, blockedTag) {
					t.Fatal("outage leaked direct")
				}
			}
		}
	}
}
