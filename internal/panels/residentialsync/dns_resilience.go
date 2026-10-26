package residentialsync

import "github.com/rezajafari0970/Digital-ocean-bot/internal/panels/resolverpolicy"

// Deployment-only canary scope; malformed/empty scopes retain the previous plan.
func DNSResilienceEnabled(panel string) bool {
	return relayScopeEnabled("DOB_DNS_RESILIENCE_PANELS", panel)
}
func (p routePolicy) udp443Only() bool { return p.DNSResilience && p.AdsOnly }
func (p routePolicy) proxyHasRoutedUDP(proxy rp) bool {
	return proxyHasUDP(proxy) || (p.udp443Only() && proxy.Type == "socks5" && proxy.UDPRequired && proxy.QUICVerified)
}
func resilientDNS() map[string]any {
	dns := managedDNS()
	dns["servers"] = resolverpolicy.ResilientTCPServers()
	dns["enableParallelQuery"] = true
	return dns
}

// Partial UDP evidence authorizes only the measured destination port. Guards
// precede public/category, donor and pool-loopback rules, retaining their exact
// identity/domain/source constraints. No unmatched protected packet goes direct.
func constrainMeasuredDatagrams(next map[string]any, p routePolicy) {
	if !p.udp443Only() {
		return
	}
	routing := next["routing"].(map[string]any)
	var rules []any
	for _, v := range routing["rules"].([]any) {
		r := v.(map[string]any)
		network, _ := r["network"].(string)
		if network != "udp" && network != "tcp,udp" {
			rules = append(rules, v)
			continue
		}
		tag, _ := r["ruleTag"].(string)
		measured := tag == "dob-route-residential-ads" || tag == "dob-route-residential-ads-udp" || tag == "dob-route-relay-ads" || tag == "dob-route-relay-ads-udp" || tag == poolPrefix+"udp-fast-ads" || tag == poolPrefix+"udp-all-ads" || tag == poolPrefix+"udp-fast" || tag == poolPrefix+"udp-all"
		if measured {
			deny := map[string]any{}
			for k, v := range r {
				deny[k] = v
			}
			delete(deny, "balancerTag")
			deny["outboundTag"] = blockedTag
			deny["network"] = "udp"
			deny["port"] = "1-442,444-65535"
			deny["ruleTag"] = tag + "-unmeasured-ports"
			rules = append(rules, deny)
		}
		rules = append(rules, v)
	}
	routing["rules"] = rules
}
