package residentialsync

import (
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"strings"
)

const poolPrefix = "dob-route-pool-"

// Two-stage selection provides real per-connection 80/20 fast/exploration bias.
// The outer random balancer selects four fast loopbacks and one exploration
// loopback. Inner balancers admit only locally observed alive endpoints. Thus a
// central monitoring outage does not restart Xray or change the data-plane pool.
func configurePool(next map[string]any, p routePolicy) error {
	routing := next["routing"].(map[string]any)
	for _, key := range []string{"observatory", "burstObservatory"} {
		if old, ok := next[key].(map[string]any); ok {
			raw, ok := old["subjectSelector"].([]any)
			if !ok {
				ss, valid := old["subjectSelector"].([]string)
				if !valid {
					return errors.New("unmanaged observatory cannot be replaced")
				}
				for _, v := range ss {
					raw = append(raw, v)
				}
			}

			for _, v := range raw {
				tag, _ := v.(string)
				if !strings.HasPrefix(tag, "residential-ads-") {
					return errors.New("unmanaged observatory cannot be replaced")
				}
			}
		}
		delete(next, key)
	}
	var outs []any
	for _, v := range next["outbounds"].([]any) {
		m := v.(map[string]any)
		tag, _ := m["tag"].(string)
		if !strings.HasPrefix(tag, "residential-ads-") && !strings.HasPrefix(tag, poolPrefix) {
			outs = append(outs, v)
		}
	}
	var balancers []any
	if old, ok := routing["balancers"].([]any); ok {
		for _, v := range old {
			m, ok := v.(map[string]any)
			if !ok {
				return errors.New("invalid balancer")
			}
			tag, _ := m["tag"].(string)
			if !strings.HasPrefix(tag, poolPrefix) {
				balancers = append(balancers, v)
			}
		}
	}
	var tcp, udp []string
	for _, proxy := range p.Proxies {
		protocol := "http"
		if proxy.Type == "socks5" {
			protocol = "socks"
			udp = append(udp, proxy.Tag)
		} else if proxy.Type != "http" && proxy.Type != "https" {
			return errors.New("unsupported residential protocol")
		}
		server := map[string]any{"address": proxy.Host, "port": proxy.Port}
		if proxy.User != "" || proxy.Password != "" {
			server["users"] = []any{map[string]any{"user": proxy.User, "pass": proxy.Password}}
		}
		o := map[string]any{"tag": proxy.Tag, "protocol": protocol, "settings": map[string]any{"servers": []any{server}}}
		if proxy.Type == "https" {
			o["streamSettings"] = map[string]any{"security": "tls", "tlsSettings": map[string]any{"serverName": proxy.Host}}
		}
		outs = append(outs, o)
		tcp = append(tcp, proxy.Tag)
	}
	var internal []any
	for _, network := range []string{"tcp", "udp"} {
		candidates := tcp
		if network == "udp" {
			candidates = udp
		}
		if len(candidates) == 0 {
			continue
		}
		for _, kind := range []string{"fast", "all"} {
			expected := len(candidates)
			if kind == "fast" {
				expected = (expected + 2) / 3
				if p.Performance != nil && p.Performance.FastCount > 0 {
					expected = min(p.Performance.FastCount, len(candidates))
				}
			}
			tag := poolPrefix + network + "-" + kind
			settings := map[string]any{"expected": expected, "maxRTT": "3s"}
			if p.Performance != nil {
				settings["maxRTT"] = residentialperf.DurationMS(p.Performance.MaxRTT)
				var costs []any
				for _, proxy := range p.Proxies {
					if v, ok := p.Performance.Costs[proxy.ID]; ok {
						costs = append(costs, map[string]any{"match": proxy.Tag, "value": v, "regexp": false})
					}
				}
				if len(costs) > 0 {
					settings["costs"] = costs
				}
			}
			balancers = append(balancers, map[string]any{"tag": tag, "selector": candidates, "fallbackTag": blockedTag, "strategy": map[string]any{"type": "leastLoad", "settings": settings}})
			inbound := []string{poolPrefix + "in-" + network + "-" + kind}
			if p.StrictAllowlist {
				// Recheck the destination after loopback, not just at the public inlet.
				internal = append(internal, map[string]any{"type": "field", "ruleTag": tag + "-dns-deny", "inboundTag": inbound, "network": network, "port": "53", "outboundTag": blockedTag})
				if !p.SniffingBlocked && p.Residential {
					internal = append(internal, map[string]any{"type": "field", "ruleTag": tag + "-ads", "inboundTag": inbound, "network": network, "domain": residentialDomains(), "balancerTag": tag})
					if network == "tcp" {
						internal = append(internal, map[string]any{"type": "field", "ruleTag": tag + "-probes", "inboundTag": inbound, "network": network, "domain": residentialProbeDomains, "port": "80,443", "balancerTag": tag})
					}
				}
				internal = append(internal, map[string]any{"type": "field", "ruleTag": tag + "-deny", "inboundTag": inbound, "network": network, "outboundTag": blockedTag})
			} else {
				internal = append(internal, map[string]any{"type": "field", "ruleTag": tag, "inboundTag": inbound, "network": network, "balancerTag": tag})
			}
		}
		lanes := []string{}
		laneCount, fastLanes := 5, 4
		if p.Performance != nil {
			laneCount = 10
			fastLanes = p.Performance.FastShare / 10
		}
		for i := 0; i < laneCount; i++ {
			kind := "fast"
			if i >= fastLanes {
				kind = "all"
			}
			tag := fmt.Sprintf("%slane-%s-%d", poolPrefix, network, i)
			outs = append(outs, map[string]any{"tag": tag, "protocol": "loopback", "settings": map[string]any{"inboundTag": poolPrefix + "in-" + network + "-" + kind}})
			lanes = append(lanes, tag)
		}
		balancers = append(balancers, map[string]any{"tag": poolPrefix + network, "selector": lanes, "fallbackTag": blockedTag, "strategy": map[string]any{"type": "random"}})
	}
	rules := []any{}
	for _, v := range routing["rules"].([]any) {
		m := v.(map[string]any)
		tag, _ := m["ruleTag"].(string)
		if strings.HasPrefix(tag, poolPrefix) {
			continue
		}
		if p.Residential && !p.SniffingBlocked && (tag == "dob-route-residential-ads" || tag == "dob-route-residential-probes" || (!p.AdsOnly && tag == "dob-route-default")) {
			for _, network := range []string{"tcp", "udp"} {
				if tag == "dob-route-residential-probes" && network != "tcp" {
					continue
				}
				copy := map[string]any{}
				for k, v := range m {
					copy[k] = v
				}
				copy["network"] = network
				copy["ruleTag"] = tag + "-" + network
				candidates := tcp
				if network == "udp" {
					candidates = udp
				}
				delete(copy, "outboundTag")
				delete(copy, "balancerTag")
				if len(candidates) > 0 {
					copy["balancerTag"] = poolPrefix + network
				} else {
					copy["outboundTag"] = blockedTag
				}
				rules = append(rules, copy)
			}
			continue
		}
		if tag == "dob-route-residential-udp" {
			continue
		}
		rules = append(rules, m)
	}
	// Sanaei hoists its API rule during SaveXraySetting. Preserve that same
	// priority before fingerprinting, otherwise exact readback differs although
	// the data-plane pool itself is valid. Never relax the hash comparison.
	apiTag := "api"
	if api, ok := next["api"].(map[string]any); ok {
		if tag, ok := api["tag"].(string); ok && tag != "" {
			apiTag = tag
		}
	}
	prefix := []any{}
	remaining := []any{}
	for _, v := range rules {
		m := v.(map[string]any)
		if m["outboundTag"] == apiTag {
			prefix = append(prefix, v)
		} else {
			remaining = append(remaining, v)
		}
	}
	routing["rules"] = append(append(prefix, internal...), remaining...)
	routing["balancers"] = balancers
	next["outbounds"] = outs
	if len(tcp) > 0 {
		next["burstObservatory"] = map[string]any{"subjectSelector": tcp, "pingConfig": map[string]any{"destination": "https://connectivitycheck.gstatic.com/generate_204", "interval": "10s", "sampling": 1, "timeout": "3s", "httpMethod": "HEAD"}}
	}
	if p.Performance != nil && len(tcp) > 0 {
		ping := next["burstObservatory"].(map[string]any)["pingConfig"].(map[string]any)
		ping["interval"] = fmt.Sprintf("%ds", p.Performance.ProbeInterval)
		ping["timeout"] = residentialperf.DurationMS(p.Performance.ProbeTimeout)
	}
	return nil
}
func fingerprintPool(next map[string]any, replacements map[string]string) {
	routing := next["routing"].(map[string]any)
	replace := func(v any) any {
		if s, ok := v.(string); ok {
			if t, ok := replacements[s]; ok {
				return t
			}
		}
		return v
	}
	for _, v := range routing["balancers"].([]any) {
		m := v.(map[string]any)
		tag, _ := m["tag"].(string)
		if !strings.HasPrefix(tag, poolPrefix) {
			continue
		}
		m["fallbackTag"] = replace(m["fallbackTag"])
		if strategy, ok := m["strategy"].(map[string]any); ok {
			if settings, ok := strategy["settings"].(map[string]any); ok {
				if costs, ok := settings["costs"].([]any); ok {
					for _, v := range costs {
						cost := v.(map[string]any)
						cost["match"] = replace(cost["match"])
					}
				}
			}
		}
		if ss, ok := m["selector"].([]string); ok {
			for i, s := range ss {
				ss[i] = replace(s).(string)
			}
		}
	}
	if obs, ok := next["burstObservatory"].(map[string]any); ok {
		if ss, ok := obs["subjectSelector"].([]string); ok {
			for i, s := range ss {
				ss[i] = replace(s).(string)
			}
		}
	}
}
func poolHasUDP(p routePolicy) bool {
	for _, x := range p.Proxies {
		if x.Type == "socks5" {
			return true
		}
	}
	return false
}
func poolLaneMatches(desired map[string]any, network, tag string) bool {
	for _, v := range desired["outbounds"].([]any) {
		o := v.(map[string]any)
		t, _ := o["tag"].(string)
		if t == tag && strings.HasPrefix(t, poolPrefix+"lane-"+network+"-") && o["protocol"] == "loopback" {
			return true
		}
	}
	return false
}

// Canary rollback must remove managed observer/balancer references before the
// old single-outbound plan loads. Unrelated operators' components are retained.
func removeManagedPool(next map[string]any) {
	routing := next["routing"].(map[string]any)
	if bs, ok := routing["balancers"].([]any); ok {
		var keep []any
		for _, v := range bs {
			m, ok := v.(map[string]any)
			if !ok {
				keep = append(keep, v)
				continue
			}
			tag, _ := m["tag"].(string)
			if !strings.HasPrefix(tag, poolPrefix) {
				keep = append(keep, v)
			}
		}
		if len(keep) > 0 {
			routing["balancers"] = keep
		} else {
			delete(routing, "balancers")
		}
	}
	for _, key := range []string{"observatory", "burstObservatory"} {
		obs, ok := next[key].(map[string]any)
		if !ok {
			continue
		}
		var tags []string
		switch ss := obs["subjectSelector"].(type) {
		case []string:
			tags = ss
		case []any:
			for _, v := range ss {
				s, _ := v.(string)
				tags = append(tags, s)
			}
		}
		managed := len(tags) > 0
		for _, tag := range tags {
			if !strings.HasPrefix(tag, "residential-ads-") {
				managed = false
			}
		}
		if managed {
			delete(next, key)
		}
	}
}
