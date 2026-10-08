package residentialsync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func strictRoute(t *testing.T, next map[string]any, inbound, email, domain, ip, network, port string) string {
	t.Helper()
	f := fakeCore{running: next}
	form := url.Values{"inboundTag": {inbound}, "email": {email}, "domain": {domain}, "ip": {ip}, "network": {network}, "port": {port}}
	resp, err := f.Do(context.Background(), routeRequest(form))
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ Obj struct{ OutboundTag string } }
	if err = json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	return out.Obj.OutboundTag
}

func TestStrictAllowlistNoOrdinaryDomainIPDNSOrProbeSuffixEscape(t *testing.T) {
	for _, kind := range []string{"socks5", "http", "down", "absent", "sniff-blocked", "residential-disabled"} {
		t.Run(kind, func(t *testing.T) {
			p := routePolicy{StrictAllowlist: true, Explicit: true, Residential: true, Direct: true, Configured: 1}
			if kind == "absent" {
				p.Configured = 0
			}
			if kind == "residential-disabled" {
				p.Residential = false
			}
			if kind == "socks5" || kind == "http" || kind == "sniff-blocked" {
				typ := kind
				if typ == "sniff-blocked" {
					typ = "socks5"
					p.SniffingBlocked = true
				}
				p.Proxies = []rp{{Type: typ, Host: "127.0.0.1", Port: 1, Tag: "residential-ads-test"}}
			}
			clients := []clientRoute{{ID: "d", Email: "d@test", Class: "DIRECT", Effective: "DIRECT"}, {ID: "r", Email: "r@test", Class: "RESIDENTIAL", Effective: "RESIDENTIAL"}, {ID: "stale", Email: "stale@test", Class: "RESIDENTIAL", Effective: "DIRECT"}}
			next, err := buildSettings(baseSettings(), clients, []string{"in"}, p)
			if err != nil {
				t.Fatal(err)
			}
			blocked := tagged(next, blockedTag)
			direct := tagged(next, directTag)
			if next["outbounds"].([]any)[0].(map[string]any)["protocol"] != "blackhole" {
				t.Fatal("unsafe global default")
			}
			for _, email := range []string{"r@test", "stale@test", "unassigned@test"} {
				for _, network := range []string{"tcp", "udp"} {
					for _, target := range []struct{ domain, ip, port string }{
						{"www.google.com", "", "443"}, {"api.ipify.org", "", "443"}, {"", "1.1.1.1", "443"},
						{"", "2001:db8::1", "443"}, {"", "8.8.8.8", "53"}, {"adservice.google.com", "", "53"},
						{"www.gstatic.com.evil.test", "", "443"}, {"evil.www.gstatic.com", "", "443"},
						{"www.gstatic.com", "", "8443"}, {"dns.google", "", "443"}, {"browserleaks.com.evil.test", "", "443"},
					} {
						if got := strictRoute(t, next, "in", email, target.domain, target.ip, network, target.port); got != blocked {
							t.Fatalf("escaped %s %s %+v => %s", email, network, target, got)
						}
					}
				}
			}
			for _, target := range []struct{ domain, ip, port string }{{"www.google.com", "", "443"}, {"", "1.1.1.1", "53"}} {
				for _, network := range []string{"tcp", "udp"} {
					if strictRoute(t, next, "in", "d@test", target.domain, target.ip, network, target.port) != direct {
						t.Fatal("direct identity affected")
					}
				}
			}
			for _, domain := range []string{"adservice.google.com", "browserleaks.com", "tls.browserleaks.com", "www.gstatic.com", "connectivitycheck.gstatic.com"} {
				for _, network := range []string{"tcp", "udp"} {
					want := blocked
					probe := strings.Contains(domain, "gstatic.com")
					if !p.SniffingBlocked && p.Residential && len(p.Proxies) > 0 && (network == "tcp" || kind == "socks5" && !probe) {
						want = tagged(next, p.Proxies[0].Tag)
					}
					if got := strictRoute(t, next, "in", "r@test", domain, "", network, "443"); got != want {
						t.Fatalf("allow %s %s got %s want %s", domain, network, got, want)
					}
				}
			}
			if strictRoute(t, next, "unobserved", "d@test", "adservice.google.com", "", "tcp", "443") != blocked {
				t.Fatal("unobserved inbound escaped")
			}
			for _, target := range []struct {
				ip, network, port string
				allow             bool
			}{{"1.1.1.1", "tcp", "53", true}, {"8.8.8.8", "tcp", "53", true}, {"9.9.9.9", "tcp", "53", false}, {"1.1.1.1", "udp", "53", false}, {"1.1.1.1", "tcp", "443", false}} {
				want := blocked
				if target.allow {
					want = direct
				}
				if strictRoute(t, next, dnsTag, "", "", target.ip, target.network, target.port) != want {
					t.Fatal("internal DNS escape", target)
				}
			}
			again, err := buildSettings(next, clients, []string{"in"}, p)
			if err != nil || settingsHash(next) != settingsHash(again) {
				t.Fatal("strict plan not stable", err)
			}
			if err := verifyRunning(context.Background(), &fakeCore{running: next}, next, clients, []string{"in"}, p); err != nil {
				t.Fatal("native verifier expectations disagree", err)
			}
		})
	}
}

func TestStrictPoolRechecksAllowlistAndRollback(t *testing.T) {
	p := routePolicy{StrictAllowlist: true, PoolEnabled: true, Residential: true, Direct: true, Proxies: []rp{{Type: "socks5", Tag: "residential-ads-one", Host: "127.0.0.1", Port: 1}}}
	next, err := buildSettings(baseSettings(), nil, []string{"in"}, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, network := range []string{"tcp", "udp"} {
		for _, kind := range []string{"fast", "all"} {
			inbound := poolPrefix + "in-" + network + "-" + kind
			for _, domain := range []string{"www.google.com", "www.gstatic.com.evil.test", ""} {
				if got := strictRoute(t, next, inbound, "", domain, "1.1.1.1", network, "443"); got != tagged(next, blockedTag) {
					t.Fatal("second stage escaped", inbound, domain, got)
				}
			}
			if got := strictRoute(t, next, inbound, "", "adservice.google.com", "", network, "53"); got != tagged(next, blockedTag) {
				t.Fatal("DNS second-stage escape")
			}
		}
	}
	again, err := buildSettings(next, nil, []string{"in"}, p)
	if err != nil || settingsHash(next) != settingsHash(again) {
		t.Fatal("unstable pool", err)
	}
	legacy := p
	legacy.StrictAllowlist = false
	legacy.AdsOnly = true
	legacy.Harden = true
	restored, err := buildSettings(next, nil, []string{"in"}, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if got := strictRoute(t, restored, "in", "", "www.google.com", "", "tcp", "443"); got != tagged(restored, directTag) {
		t.Fatal("rollback not selective")
	}
	for _, v := range restored["routing"].(map[string]any)["rules"].([]any) {
		tag, _ := v.(map[string]any)["ruleTag"].(string)
		if strings.Contains(tag, "-probes") || strings.Contains(tag, "-dns-deny") {
			t.Fatal("strict rule survived rollback", tag)
		}
	}
}

func TestStrictSniffingCannotRetainArbitraryIP(t *testing.T) {
	raw := json.RawMessage(`{"protocol":"vless","sniffing":{"enabled":true,"destOverride":["http","tls","quic"],"routeOnly":true}}`)
	if validateAdSniffing([]json.RawMessage{raw}, true) == nil {
		t.Fatal("routeOnly bypass accepted")
	}
	if validateAdSniffing([]json.RawMessage{raw}, false) != nil {
		t.Fatal("legacy rollback changed")
	}
}
func TestStrictAllowlistRolloutScope(t *testing.T) {
	const a = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const b = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	const c = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	for _, scope := range []string{"", "all", "none", a + ", " + b, "typo", "none,garbage", a + ",", "aaaaaaaa"} {
		t.Setenv("DOB_RESIDENTIAL_ALLOWLIST_PANELS", scope)
		expected := scope != "none" && scope != a+", "+b
		if got := StrictAllowlistEnabled(c); got != expected {
			t.Fatal(scope, got, expected)
		}
	}
	t.Setenv("DOB_RESIDENTIAL_ALLOWLIST_PANELS", strings.ToUpper(a)+", "+b)
	if !StrictAllowlistEnabled(a) || !StrictAllowlistEnabled(b) || StrictAllowlistEnabled(c) {
		t.Fatal("validated canary scope")
	}
}

func TestStrictRejectsInfrastructureInboundCollisionsBeforeBuild(t *testing.T) {
	p := routePolicy{StrictAllowlist: true, Residential: true, Direct: true}
	for _, tag := range []string{dnsTag, poolPrefix + "in-tcp-fast", "api"} {
		raw := json.RawMessage(fmt.Sprintf("{\"id\":1,\"tag\":%q,\"protocol\":\"vless\",\"settings\":{\"clients\":[]}}", tag))
		if _, _, e := planClients([]json.RawMessage{raw}, nil, p); e == nil {
			t.Fatal("reserved inventory accepted", tag)
		}
		if _, e := buildSettings(baseSettings(), nil, []string{tag}, p); e == nil {
			t.Fatal("reserved build accepted", tag)
		}
	}
	current := baseSettings()
	current["api"] = map[string]any{"tag": "custom-admin"}
	if _, e := buildSettings(current, nil, []string{"custom-admin"}, p); e == nil {
		t.Fatal("custom API collision")
	}
	current = baseSettings()
	current["inbounds"] = []any{map[string]any{"tag": "custom-internal"}}
	if _, e := buildSettings(current, nil, []string{"custom-internal"}, p); e == nil {
		t.Fatal("template inbound collision")
	}
	if _, e := buildSettings(current, nil, []string{"ordinary-client"}, p); e != nil {
		t.Fatal(e)
	}
}
