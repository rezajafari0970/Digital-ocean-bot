package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func sameCategoryStrings(value any, expected []string) bool {
	a, err := json.Marshal(value)
	if err != nil {
		return false
	}
	b, _ := json.Marshal(expected)
	return string(a) == string(b)
}

// Exact inner policy is checked independently of observed pool health. A
// fallback blackhole remains valid fail-closed behavior but is never positive
// evidence that a newly allowed host reaches a residential outbound.
func validateCategoryContract(setting map[string]any, wanted []string, strict bool, pool map[string][]string, clientPaths bool, inboundTags []string) error {
	routing, _ := setting["routing"].(map[string]any)
	rules, _ := routing["rules"].([]any)
	blocked := ""
	outs := map[string]map[string]any{}
	for _, value := range setting["outbounds"].([]any) {
		out, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid outbound")
		}
		tag, _ := out["tag"].(string)
		outs[tag] = out
		if strings.HasPrefix(tag, "dob-route-blocked-") {
			if blocked != "" || out["protocol"] != "blackhole" {
				return fmt.Errorf("unverified blocked outbound")
			}
			blocked = tag
		}
	}
	if blocked == "" {
		return fmt.Errorf("blocked outbound missing")
	}
	adsIndex, udpIndex, udpCount := -1, -1, 0
	var ads, udp map[string]any
	for i, value := range rules {
		rule, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid category rule")
		}
		if rule["ruleTag"] == "dob-route-residential-ads" {
			if adsIndex >= 0 {
				return fmt.Errorf("duplicate shared category rule")
			}
			adsIndex, ads = i, rule
		}
		if rule["ruleTag"] == "dob-route-residential-udp" {
			udpIndex, udp = i, rule
			udpCount++
		}
	}
	// Verify the full public rule independently of sampled routeTest identities.
	// A residential-looking name cannot establish an actual proxy transport.
	if len(inboundTags) == 0 {
		return fmt.Errorf("no observed client inbound")
	}
	outerCount := 0
	seenOuter := map[string]bool{}
	for _, value := range rules {
		rule := value.(map[string]any)
		tag, _ := rule["ruleTag"].(string)
		if tag != "dob-route-residential-ads" && tag != "dob-route-residential-ads-tcp" && tag != "dob-route-residential-ads-udp" {
			continue
		}
		outerCount++
		if seenOuter[tag] || len(rule) != 6 || rule["type"] != "field" || !sameCategoryStrings(rule["inboundTag"], inboundTags) || !sameCategoryStrings(rule["domain"], wanted) {
			return fmt.Errorf("public category scope differs")
		}
		seenOuter[tag] = true
		if len(pool) == 0 {
			target, _ := rule["outboundTag"].(string)
			protocol := outs[target]["protocol"]
			if tag != "dob-route-residential-ads" || rule["network"] != "tcp,udp" || !strings.HasPrefix(target, "residential-ads-") || (protocol != "http" && protocol != "socks") {
				return fmt.Errorf("public residential transport unverified")
			}
		} else {
			network, _ := rule["network"].(string)
			if (network != "tcp" && network != "udp") || tag != "dob-route-residential-ads-"+network {
				return fmt.Errorf("public pool network differs")
			}
			if len(pool["@inner:"+network+":all"]) > 1 {
				if rule["balancerTag"] != "dob-route-pool-"+network {
					return fmt.Errorf("public pool binding differs")
				}
			} else if network != "udp" || rule["outboundTag"] != blocked {
				return fmt.Errorf("public pool lacks positive TCP transport")
			}
		}
	}
	if (len(pool) == 0 && outerCount != 1) || (len(pool) > 0 && (outerCount != 2 || !seenOuter["dob-route-residential-ads-tcp"] || !seenOuter["dob-route-residential-ads-udp"])) {
		return fmt.Errorf("public category rule missing")
	}
	httpPlan := false
	if ads != nil {
		target, _ := ads["outboundTag"].(string)
		httpPlan = outs[target]["protocol"] == "http"
	}
	if httpPlan || udpCount > 0 {
		if !httpPlan || udpCount != 1 || udpIndex >= adsIndex || len(udp) != 6 || udp["type"] != "field" || udp["network"] != "udp" || udp["outboundTag"] != blocked || !sameCategoryStrings(udp["domain"], wanted) {
			return fmt.Errorf("HTTP UDP category deny differs")
		}
		a, _ := json.Marshal(ads["inboundTag"])
		u, _ := json.Marshal(udp["inboundTag"])
		if string(a) == "null" || string(a) == "[]" || string(a) != string(u) {
			return fmt.Errorf("HTTP UDP inbound differs")
		}
	}
	if !strict {
		return nil
	}
	for _, network := range []string{"tcp", "udp"} {
		for _, kind := range []string{"fast", "all"} {
			if len(pool["@inner:"+network+":"+kind]) == 0 {
				continue
			}
			prefix := "dob-route-pool-" + network + "-" + kind
			inbound := "dob-route-pool-in-" + network + "-" + kind
			categoryIndex, denyIndex, count := -1, -1, 0
			for i, value := range rules {
				rule := value.(map[string]any)
				tag, _ := rule["ruleTag"].(string)
				if tag == prefix+"-ads" {
					count++
					categoryIndex = i
					if len(rule) != 6 || rule["type"] != "field" || rule["network"] != network || !sameCategoryStrings(rule["inboundTag"], []string{inbound}) || !sameCategoryStrings(rule["domain"], wanted) || rule["balancerTag"] != prefix || rule["outboundTag"] != nil {
						return fmt.Errorf("inner category contract differs: %s", prefix)
					}
					for _, key := range []string{"port", "ip", "user", "protocol", "source", "sourcePort", "attrs"} {
						if _, ok := rule[key]; ok {
							return fmt.Errorf("unexpected inner category restriction")
						}
					}
				}
				if tag == prefix+"-deny" {
					denyIndex = i
				}
			}
			if count != 1 || categoryIndex < 0 || denyIndex <= categoryIndex {
				return fmt.Errorf("missing, duplicate or shadowed inner category rule: %s", prefix)
			}
			dnsCount, infrastructureCount := 0, 0
			for i := 0; i < categoryIndex; i++ {
				rule := rules[i].(map[string]any)
				tag, _ := rule["ruleTag"].(string)
				encoded, _ := json.Marshal(rule["inboundTag"])
				applies := rule["inboundTag"] == nil || strings.Contains(string(encoded), `"`+inbound+`"`)
				if !applies {
					continue
				}
				outbound, _ := rule["outboundTag"].(string)
				if len(rule) != 6 || rule["type"] != "field" || rule["network"] != network || !sameCategoryStrings(rule["inboundTag"], []string{inbound}) || outbound != blocked {
					return fmt.Errorf("unexpected earlier inner rule")
				}
				switch tag {
				case prefix + "-dns-deny":
					dnsCount++
					if rule["port"] != "53" {
						return fmt.Errorf("inner DNS precedence differs")
					}
				case prefix + "-infrastructure-deny":
					infrastructureCount++
					if !sameCategoryStrings(rule["domain"], []string{"full:www.gstatic.com", "full:connectivitycheck.gstatic.com", "full:www.google.com", "full:cloudflare-dns.com"}) {
						return fmt.Errorf("inner infrastructure precedence differs")
					}
				default:
					return fmt.Errorf("unexpected earlier inner rule")
				}
			}
			if dnsCount != 1 || (clientPaths && infrastructureCount != 1) || (!clientPaths && infrastructureCount != 0) {
				return fmt.Errorf("missing or duplicate inner precedence guard: %s", prefix)
			}
		}
	}
	return nil
}

func positivePoolTargets(pool map[string][]string) map[string][]string {
	out := map[string][]string{}
	for key, values := range pool {
		if !strings.HasPrefix(key, "@inner:") {
			continue
		}
		for _, value := range values {
			if strings.HasPrefix(value, "residential-ads-") {
				out["@positive:"+strings.TrimPrefix(key, "@inner:")] = append(out["@positive:"+strings.TrimPrefix(key, "@inner:")], value)
			}
		}
	}
	return out
}
