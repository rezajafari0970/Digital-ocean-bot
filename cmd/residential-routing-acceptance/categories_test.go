package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func categoryContractFixture() (map[string]any, map[string][]string, []string) {
	wanted := []string{"geosite:google@ads", "geosite:youtube"}
	rules := []any{}
	pool := map[string][]string{}
	for _, netw := range []string{"tcp", "udp"} {
		for _, kind := range []string{"fast", "all"} {
			tag := "dob-route-pool-" + netw + "-" + kind
			in := []string{"dob-route-pool-in-" + netw + "-" + kind}
			pool["@inner:"+netw+":"+kind] = []string{"dob-route-blocked-hash", "residential-ads-test-hash"}
			rules = append(rules,
				map[string]any{"type": "field", "ruleTag": tag + "-dns-deny", "inboundTag": in, "network": netw, "port": "53", "outboundTag": "dob-route-blocked-hash"},
				map[string]any{"type": "field", "ruleTag": tag + "-infrastructure-deny", "inboundTag": in, "network": netw, "domain": []string{"full:www.gstatic.com", "full:connectivitycheck.gstatic.com", "full:www.google.com", "full:cloudflare-dns.com"}, "outboundTag": "dob-route-blocked-hash"},
				map[string]any{"type": "field", "ruleTag": tag + "-ads", "inboundTag": in, "network": netw, "domain": wanted, "balancerTag": tag}, map[string]any{"type": "field", "ruleTag": tag + "-deny", "inboundTag": in, "network": netw, "outboundTag": "dob-route-blocked-hash"})
		}
	}
	for _, network := range []string{"tcp", "udp"} {
		rules = append(rules, map[string]any{"type": "field", "ruleTag": "dob-route-residential-ads-" + network, "network": network, "inboundTag": []string{"in443"}, "domain": wanted, "balancerTag": "dob-route-pool-" + network})
	}
	return map[string]any{"outbounds": []any{map[string]any{"tag": "dob-route-blocked-hash", "protocol": "blackhole"}}, "routing": map[string]any{"rules": rules}}, pool, wanted
}
func TestExpandedContractRejectsMissingLegacyOrShadowedInnerScope(t *testing.T) {
	for _, kind := range []string{"valid", "missing", "legacy", "shadowed", "binding", "extra_restriction", "http_udp", "missing_dns", "reordered_dns", "duplicate_dns", "missing_infra", "dns_direct"} {
		t.Run(kind, func(t *testing.T) {
			s, pool, wanted := categoryContractFixture()
			routing := s["routing"].(map[string]any)
			rules := routing["rules"].([]any)
			first := rules[2].(map[string]any)
			switch kind {
			case "missing":
				rules = append(rules[:2], rules[3:]...)
			case "legacy":
				first["domain"] = []string{"geosite:google@ads"}
			case "shadowed":
				rules[2], rules[3] = rules[3], rules[2]
			case "binding":
				first["balancerTag"] = "direct"
			case "extra_restriction":
				first["port"] = "80"
			case "missing_dns":
				rules = rules[1:]
			case "reordered_dns":
				rules[0], rules[2] = rules[2], rules[0]
			case "duplicate_dns":
				rules = append([]any{rules[0]}, rules...)
			case "missing_infra":
				rules = append(rules[:1], rules[2:]...)
			case "dns_direct":
				rules[0].(map[string]any)["outboundTag"] = "direct"
			case "http_udp":
				rules = append(rules, map[string]any{"ruleTag": "dob-route-residential-udp", "domain": []string{"geosite:google@ads"}})
			}
			routing["rules"] = rules
			err := validateCategoryContract(s, wanted, true, pool, true, []string{"in443"})
			if (kind == "valid") != (err == nil) {
				t.Fatalf("%s: %v", kind, err)
			}
		})
	}
	_, pool, _ := categoryContractFixture()
	for _, values := range positivePoolTargets(pool) {
		if len(values) != 1 || !strings.HasPrefix(values[0], "residential-ads-") {
			t.Fatal("blocked counted as positive")
		}
	}
}
func TestExpandedProofRespectsStrictAndSelectiveFallback(t *testing.T) {
	for _, unmatched := range []string{"blocked", "direct"} {
		calls := 0
		_, pool, _ := categoryContractFixture()
		for k, v := range positivePoolTargets(pool) {
			pool[k] = v
		}
		_, err := verifyExpandedCategories(func(form url.Values, want string) error {
			calls++
			host := form.Get("domain")
			if host == "huawei.com" || host == "gog.com" || strings.HasSuffix(host, ".evil.test") {
				if want != unmatched {
					t.Fatalf("bad unmatched expectation: %s", want)
				}
			}
			if strings.Contains(form.Get("inboundTag"), "pool-in-") && form.Get("port") != "53" && !strings.HasPrefix(want, "@positive:") {
				t.Fatal("inner proof admits fallback")
			}
			return nil
		}, "in", map[string]string{"DIRECT": "direct-user", "RESIDENTIAL": "res-user"}, "direct", unmatched, "protected", "", pool, unmatched == "blocked", "blocked")
		expected := 100
		if unmatched == "blocked" {
			expected += 8
		}
		if err != nil || calls != expected {
			t.Fatal(calls, err)
		}
	}
}

func TestHTTPUDPContractRejectsDirectRestrictedReorderedAndMissing(t *testing.T) {
	for _, mutation := range []string{"valid", "direct", "port", "inbound", "reordered", "missing", "domain", "not_blackhole"} {
		t.Run(mutation, func(t *testing.T) {
			wanted := []string{"geosite:youtube"}
			in := []string{"in443"}
			outs := []any{map[string]any{"tag": "dob-route-blocked-hash", "protocol": "blackhole"}, map[string]any{"tag": "residential-ads-http", "protocol": "http"}}
			deny := map[string]any{"type": "field", "ruleTag": "dob-route-residential-udp", "inboundTag": in, "network": "udp", "domain": wanted, "outboundTag": "dob-route-blocked-hash"}
			ads := map[string]any{"type": "field", "ruleTag": "dob-route-residential-ads", "inboundTag": in, "network": "tcp,udp", "domain": wanted, "outboundTag": "residential-ads-http"}
			rules := []any{deny, ads}
			switch mutation {
			case "direct":
				deny["outboundTag"] = "direct"
			case "port":
				deny["port"] = "443"
			case "inbound":
				deny["inboundTag"] = []string{"wrong"}
			case "reordered":
				rules = []any{ads, deny}
			case "missing":
				rules = []any{ads}
			case "domain":
				deny["domain"] = []string{"geosite:google@ads"}
			case "not_blackhole":
				outs[0].(map[string]any)["protocol"] = "freedom"
			}
			setting := map[string]any{"outbounds": outs, "routing": map[string]any{"rules": rules}}
			err := validateCategoryContract(setting, wanted, true, nil, true, []string{"in443"})
			if (mutation == "valid") != (err == nil) {
				t.Fatal(mutation, err)
			}
		})
	}
	var checked bool
	_, err := verifyExpandedCategories(func(form url.Values, want string) error {
		if form.Get("network") == "udp" && form.Get("port") == "8443" {
			checked = true
			if want != "blocked" {
				t.Fatal(want)
			}
		}
		return nil
	}, "in", map[string]string{"DIRECT": "d", "RESIDENTIAL": "r"}, "direct", "blocked", "protected", "blocked", nil, true, "blocked")
	if err != nil || !checked {
		t.Fatal(checked, err)
	}
}

func TestPoolPositiveSelectorsRequireActualSupportedOutbound(t *testing.T) {
	for _, protocol := range []string{"socks", "http", "freedom", "blackhole", "missing"} {
		t.Run(protocol, func(t *testing.T) {
			blocked := "dob-route-blocked-hash"
			proxy := "residential-ads-test"
			outs := []any{map[string]any{"tag": blocked, "protocol": "blackhole"}}
			if protocol != "missing" {
				outs = append(outs, map[string]any{"tag": proxy, "protocol": protocol})
			}
			balancers := []any{}
			for _, network := range []string{"tcp", "udp"} {
				// HTTP is deliberately tested in TCP only; UDP has no available pool.
				if protocol == "http" && network == "udp" {
					continue
				}
				lanes := []string{}
				for i := 0; i < 5; i++ {
					kind := "fast"
					if i == 4 {
						kind = "all"
					}
					tag := "lane-" + network + string(rune('a'+i))
					lanes = append(lanes, tag)
					outs = append(outs, map[string]any{"tag": tag, "protocol": "loopback", "settings": map[string]any{"inboundTag": "dob-route-pool-in-" + network + "-" + kind}})
				}
				balancers = append(balancers, map[string]any{"tag": "dob-route-pool-" + network, "selector": lanes, "fallbackTag": blocked, "strategy": map[string]any{"type": "random"}})
				for _, kind := range []string{"fast", "all"} {
					balancers = append(balancers, map[string]any{"tag": "dob-route-pool-" + network + "-" + kind, "selector": []string{proxy}, "fallbackTag": blocked, "strategy": map[string]any{"type": "leastLoad", "settings": map[string]any{"expected": 1, "maxRTT": "3s"}}})
				}
			}
			s := map[string]any{"outbounds": outs, "routing": map[string]any{"balancers": balancers}, "burstObservatory": map[string]any{"subjectSelector": []string{proxy}, "pingConfig": map[string]any{"sampling": 1, "interval": "10s", "timeout": "3s", "destination": "https://connectivitycheck.gstatic.com/generate_204"}}}
			raw, _ := json.Marshal(s)
			json.Unmarshal(raw, &s)
			_, err := poolTargets(s)
			valid := protocol == "socks" || protocol == "http"
			if valid != (err == nil) {
				t.Fatal(protocol, err)
			}
		})
	}
}

func TestPublicCategoryContractRejectsRestrictionsAndSpoofedTransports(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		for _, mutation := range []string{"valid", "port", "user", "inbound", "network", "target", "missing", "freedom", "blackhole", "missing_outbound"} {
			t.Run(fmt.Sprintf("pool=%t/%s", pooled, mutation), func(t *testing.T) {
				s, pool, wanted := categoryContractFixture()
				routing := s["routing"].(map[string]any)
				rules := routing["rules"].([]any)
				if !pooled {
					pool = nil
					rules = []any{map[string]any{"type": "field", "ruleTag": "dob-route-residential-ads", "network": "tcp,udp", "inboundTag": []string{"in443"}, "domain": wanted, "outboundTag": "residential-ads-single"}}
					s["outbounds"] = append(s["outbounds"].([]any), map[string]any{"tag": "residential-ads-single", "protocol": "socks"})
				}
				outer := rules[len(rules)-1].(map[string]any)
				switch mutation {
				case "port":
					outer["port"] = "443"
				case "user":
					outer["user"] = []string{"sampled-only"}
				case "inbound":
					outer["inboundTag"] = []string{"other"}
				case "network":
					outer["network"] = "tcp"
				case "target":
					if pooled {
						outer["balancerTag"] = "direct"
					} else {
						outer["outboundTag"] = "direct"
					}
				case "missing":
					rules = rules[:len(rules)-1]
				case "freedom", "blackhole", "missing_outbound":
					if pooled {
						return
					}
					if mutation == "missing_outbound" {
						s["outbounds"] = s["outbounds"].([]any)[:1]
					} else {
						s["outbounds"].([]any)[1].(map[string]any)["protocol"] = mutation
					}
				}
				routing["rules"] = rules
				err := validateCategoryContract(s, wanted, true, pool, true, []string{"in443"})
				if (mutation == "valid") != (err == nil) {
					t.Fatal(mutation, err)
				}
			})
		}
	}
}
