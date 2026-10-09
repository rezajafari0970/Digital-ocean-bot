package residentialsync

import (
	"encoding/json"
	"testing"
)

func TestClientInfrastructureDirectDuringResidentialOutage(t *testing.T) {
	for _, pool := range []bool{false, true} {
		p := routePolicy{StrictAllowlist: true, PoolEnabled: pool, Residential: true, Direct: true, Configured: 1, StableFingerprint: true}
		clients := []clientRoute{{ID: "d", Email: "d@test", Class: "DIRECT", Effective: "DIRECT"}, {ID: "r", Email: "r@test", Class: "RESIDENTIAL", Effective: "BLOCKED"}}
		next, e := buildSettings(baseSettings(), clients, []string{"in"}, p)
		if e != nil {
			t.Fatal(e)
		}
		for _, email := range []string{"r@test", "new-authenticated@test"} {
			for _, host := range []string{"www.gstatic.com", "connectivitycheck.gstatic.com", "www.google.com"} {
				for _, port := range []string{"80", "443"} {
					if got := strictRoute(t, next, "in", email, host, "", "tcp", port); got != tagged(next, directTag) {
						t.Fatalf("probe must be direct independent of pool: %s:%s => %s", host, port, got)
					}
				}
			}
			if strictRoute(t, next, "in", email, "cloudflare-dns.com", "", "tcp", "443") != tagged(next, directTag) {
				t.Fatal("client DoH blocked")
			}
			for _, network := range []string{"tcp", "udp"} {
				for _, ip := range []string{"1.1.1.1", "8.8.8.8"} {
					if strictRoute(t, next, "in", email, "", ip, network, "53") != tagged(next, clientDNSTag) {
						t.Fatal("client DNS not parsed", network, ip)
					}
				}
			}
			for _, host := range []string{"adservice.google.com", "browserleaks.com", "example.com"} {
				if strictRoute(t, next, "in", email, host, "", "tcp", "443") != tagged(next, blockedTag) {
					t.Fatal("outage/non-category escaped", host)
				}
			}
		}
		old := settingsHash(next)
		clients[1].Email = "replacement@test"
		again, e := buildSettings(next, clients, []string{"in"}, p)
		if e != nil || settingsHash(again) != old {
			t.Fatal("residential turnover reloads", e)
		}
	}
}
func TestStrictInfrastructureBoundsAndDisablement(t *testing.T) {
	for _, disabled := range []string{"no", "admin", "sniff"} {
		p := routePolicy{StrictAllowlist: true, Residential: disabled != "admin", Direct: true, SniffingBlocked: disabled == "sniff"}
		next, e := buildSettings(baseSettings(), nil, []string{"in"}, p)
		if e != nil {
			t.Fatal(e)
		}
		blocked := tagged(next, blockedTag)
		for _, in := range []string{"in", "unobserved"} {
			for _, x := range []struct{ host, ip, network, port string }{{"www.google.com.evil.test", "", "tcp", "443"}, {"www.gstatic.com", "", "tcp", "8443"}, {"www.google.com", "", "udp", "443"}, {"cloudflare-dns.com", "", "tcp", "80"}, {"cloudflare-dns.com", "", "udp", "443"}, {"evil.cloudflare-dns.com", "", "tcp", "443"}, {"dns.google", "", "tcp", "443"}, {"", "1.1.1.1", "tcp", "443"}, {"", "9.9.9.9", "udp", "53"}, {"example.com", "", "tcp", "443"}, {"adservice.google.com", "", "tcp", "53"}} {
				if strictRoute(t, next, in, "new@test", x.host, x.ip, x.network, x.port) != blocked {
					t.Fatal("bounded exception escaped", disabled, in, x)
				}
			}
		}
		if disabled != "no" {
			for _, x := range []struct{ host, ip, port string }{{"www.gstatic.com", "", "443"}, {"cloudflare-dns.com", "", "443"}, {"", "1.1.1.1", "53"}} {
				if strictRoute(t, next, "in", "r@test", x.host, x.ip, "tcp", x.port) != blocked {
					t.Fatal("disablement bypass", disabled, x)
				}
			}
		}
	}
}
func TestStrictEligibleInletsOnly(t *testing.T) {
	p := routePolicy{StrictAllowlist: true, Residential: true}
	raws := []json.RawMessage{json.RawMessage(`{"id":1,"tag":"public-socks","protocol":"socks","settings":{"auth":"noauth"}}`), json.RawMessage(`{"id":2,"tag":"vless","protocol":"vless","settings":{"clients":[{"id":"id","email":"r@test"}]}}`)}
	_, tags, e := planClients(raws, nil, p)
	if e != nil || len(tags) != 1 || tags[0] != "vless" {
		t.Fatal("unauthenticated inlet eligible", tags, e)
	}
}

func TestClientPathsRolloutSelector(t *testing.T) {
	panel := "ca839a3a-d181-4e01-bd97-77da6533f399"
	for _, tc := range []struct {
		scope string
		want  bool
	}{
		{"", true}, {"all", true}, {"none", false}, {panel, true},
		{"00000000-0000-0000-0000-000000000000", false},
		{panel + ",invalid", false}, {panel + ",", false}, {" " + panel + " ", true},
	} {
		t.Run(tc.scope, func(t *testing.T) {
			t.Setenv("DOB_RESIDENTIAL_CLIENT_PATHS_PANELS", tc.scope)
			if got := ClientPathsEnabled(panel); got != tc.want {
				t.Fatal(tc.scope, got)
			}
		})
	}
}
