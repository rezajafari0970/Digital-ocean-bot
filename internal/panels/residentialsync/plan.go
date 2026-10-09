package residentialsync

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"sort"
	"strings"
)

const directTag = "dob-route-direct"
const blockedTag = "dob-route-blocked"

// Original Google advertising category; expanded authorized scope is staged in categories.go.
var adDomains = []string{"geosite:google@ads"}

// Temporary user-requested diagnostic exception, separate from Ads categories.
// domain: includes the apex and subdomains, never lookalike suffixes.
var residentialTestDomains = []string{"domain:browserleaks.com"}

func residentialDomains() []string {
	return append(append([]string{}, adDomains...), residentialTestDomains...)
}

type clientRoute struct{ ID, Email, Class, Effective string }
type routePolicy struct {
	RelayMode             bool
	RelayInlet            *relayInlet
	StrictAllowlist       bool
	ExpandedCategories    bool
	LegacyClientPaths     bool
	StableFingerprint     bool
	Performance           *residentialperf.Config
	PerformanceGeneration int64
	PerformanceBaseline   *residentialperf.Fields
	PoolEnabled           bool
	Explicit              bool
	Residential, Direct   bool
	AdsOnly               bool
	Harden                bool
	SniffingBlocked       bool
	Configured            int
	HealthyCount          int
	Proxies               []rp
}
type rp struct {
	Relay                                                *relayTransport
	TransportHash, DonorDroplet, DonorAccount, DonorPlan string
	ID, Type, Host, User, Tag, Password                  string
	Port                                                 int
}

func decodeObject(raw json.RawMessage) (map[string]any, error) {
	var nested string
	if json.Unmarshal(raw, &nested) == nil {
		raw = []byte(nested)
	}
	var out map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, errors.New("missing JSON object")
	}
	return out, nil
}
func planClients(raws []json.RawMessage, previous map[string]clientRoute, p routePolicy) ([]clientRoute, []string, error) {
	p = p.normalized()
	ids := map[string]string{}
	emails := map[string]string{}
	tags := []string{}
	seenTags := map[string]bool{}
	for _, raw := range raws {
		var in struct {
			ID       int64
			Tag      string
			Protocol string
			Settings json.RawMessage
		}
		if json.Unmarshal(raw, &in) != nil || in.ID <= 0 || in.Tag == "" {
			return nil, nil, errors.New("inbound routing identity unavailable")
		}
		if seenTags[in.Tag] {
			return nil, nil, errors.New("duplicate inbound routing tag")
		}
		seenTags[in.Tag] = true
		if p.StrictAllowlist {
			if err := validateStrictInboundTags([]string{in.Tag}, nil); err != nil {
				return nil, nil, err
			}
			// These exported subscription protocols authenticate current UUID clients.
			// Other inlets must retain the terminal deny instead of gaining DNS/probes.
			if !p.LegacyClientPaths && in.Protocol != "vless" && in.Protocol != "vmess" {
				continue
			}
		}
		tags = append(tags, in.Tag)
		if in.Protocol != "vless" && in.Protocol != "vmess" && in.Protocol != "trojan" && in.Protocol != "shadowsocks" {
			continue
		}
		settings, err := decodeObject(in.Settings)
		if err != nil {
			return nil, nil, err
		}
		values, ok := settings["clients"].([]any)
		if !ok {
			return nil, nil, errors.New("client inventory unavailable")
		}
		for _, value := range values {
			m, ok := value.(map[string]any)
			if !ok {
				return nil, nil, errors.New("invalid client")
			}
			id, _ := m["id"].(string)
			email, _ := m["email"].(string)
			// Non-UUID protocols remain under the inbound default; Output exports VLESS.
			if in.Protocol != "vless" && in.Protocol != "vmess" {
				continue
			}
			if id == "" || email == "" || strings.HasPrefix(email, "regexp:") {
				return nil, nil, errors.New("unsafe client routing identity")
			}
			if old, ok := ids[id]; ok && old != email {
				return nil, nil, errors.New("UUID/email conflict")
			}
			key := strings.ToLower(email)
			if old, ok := emails[key]; ok && old != id {
				return nil, nil, errors.New("email/UUID conflict")
			}
			ids[id] = email
			emails[key] = id
		}
	}
	if p.StrictAllowlist {
		if err := validateStrictInboundTags(tags, nil); err != nil {
			return nil, nil, err
		}
	}
	sort.Strings(tags)
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	wantedDirect := 0
	if p.Direct {
		wantedDirect = len(ids)
		if p.Residential {
			wantedDirect = len(ids) / 2
		}
	}
	out := make([]clientRoute, 0, len(ids))
	direct := 0
	var unassigned []int
	for _, id := range ordered {
		cl := clientRoute{ID: id, Email: ids[id], Class: "RESIDENTIAL"}
		old, known := previous[id]
		known = known && old.Email == cl.Email && (old.Class == "DIRECT" || old.Class == "RESIDENTIAL")
		switch {
		case p.Explicit && known:
			cl.Class = old.Class
		case p.Explicit:
			// Unknown identities remain protected; never infer Direct by count.
			cl.Class = "RESIDENTIAL"
		case p.Direct && !p.Residential:
			cl.Class = "DIRECT"
		case p.Direct && p.Residential && known:
			cl.Class = old.Class
		case p.Direct && p.Residential:
			unassigned = append(unassigned, len(out))
		}
		if cl.Class == "DIRECT" {
			direct++
		}
		out = append(out, cl)
	}
	// Both enabled: retain existing identity classes across partial deletion,
	// replacement and growth. Fill the desired split using new identities only.
	for _, i := range unassigned {
		if direct < wantedDirect {
			out[i].Class = "DIRECT"
			direct++
		}
	}

	for i := range out {
		out[i].Effective = out[i].Class
		if !p.Direct && !p.Residential {
			out[i].Effective = "BLOCKED"
		} else if out[i].Class == "RESIDENTIAL" {
			if p.SniffingBlocked {
				out[i].Effective = "BLOCKED"
			} else if p.Configured == 0 && !p.Harden && !p.AdsOnly {
				out[i].Effective = "DIRECT"
			} else if len(p.Proxies) == 0 {
				out[i].Effective = "BLOCKED"
			}
		}
	}
	return out, tags, nil
}
func managedTag(tag string) bool {
	return strings.HasPrefix(tag, "residential-ads-") || strings.HasPrefix(tag, "dob-route-")
}
func managedRule(tag string) bool {
	return tag == "dob-residential-ads" || strings.HasPrefix(tag, "dob-route-")
}
func settingsHash(v map[string]any) string {
	raw, _ := json.Marshal(v)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func buildSettings(current map[string]any, clients []clientRoute, tags []string, p routePolicy) (map[string]any, error) {
	p = p.normalized()
	if p.StrictAllowlist {
		if err := validateStrictInboundTags(tags, current); err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	next, err := decodeObject(raw)
	if err != nil {
		return nil, err
	}
	outs, ok := next["outbounds"].([]any)
	if !ok {
		return nil, errors.New("outbound inventory unavailable")
	}
	if p.PerformanceBaseline != nil {
		residentialperf.Restore(next, *p.PerformanceBaseline)
	}
	kept := []any{}
	blackholes := map[string]bool{}
	for _, value := range outs {
		m, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("invalid outbound")
		}
		tag, _ := m["tag"].(string)
		protocol, _ := m["protocol"].(string)
		if managedTag(tag) {
			continue
		}
		kept = append(kept, value)
		if protocol == "blackhole" {
			blackholes[tag] = true
		}
	}
	kept = append(kept, map[string]any{"tag": directTag, "protocol": "freedom", "settings": map[string]any{}}, map[string]any{"tag": blockedTag, "protocol": "blackhole", "settings": map[string]any{}})
	destination := blockedTag
	if p.Configured == 0 && !p.Harden && !p.AdsOnly {
		destination = directTag
	} else if len(p.Proxies) > 0 {
		proxy := p.Proxies[0]
		outbound, err := proxyOutbound(proxy)
		if err != nil {
			return nil, err
		}
		kept = append(kept, outbound)
		destination = proxy.Tag
	}
	if p.SniffingBlocked {
		destination = blockedTag
	}
	if p.Harden && p.Residential && p.AdsOnly && (!p.StrictAllowlist || !p.LegacyClientPaths && !p.SniffingBlocked) {
		// Parse client DNS locally: A/AAAA use the cached IPv4-only pool.
		// Strict mode never forwards non-IP queries or arbitrary port-53 bytes.
		nonIP := "skip"
		if p.StrictAllowlist {
			nonIP = "drop"
		}
		kept = append(kept, map[string]any{
			"tag": clientDNSTag, "protocol": "dns",
			"settings":       map[string]any{"network": "tcp", "address": "1.1.1.1", "port": 53, "nonIPQuery": nonIP},
			"streamSettings": map[string]any{"sockopt": map[string]any{"dialerProxy": directTag}},
		})
	}
	next["outbounds"] = kept
	routing, ok := next["routing"].(map[string]any)
	if !ok {
		return nil, errors.New("routing unavailable")
	}
	rules, ok := routing["rules"].([]any)
	if !ok {
		return nil, errors.New("routing rules unavailable")
	}
	first := []any{}
	rest := []any{}
	apiTag := ""
	if api, ok := next["api"].(map[string]any); ok {
		apiTag, _ = api["tag"].(string)
	}
	for _, value := range rules {
		m, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("invalid routing rule")
		}
		tag, _ := m["ruleTag"].(string)
		if managedRule(tag) {
			continue
		}
		outbound, _ := m["outboundTag"].(string)
		if (apiTag != "" && outbound == apiTag) || (blackholes[outbound] && (!p.AdsOnly || destinationIPGuard(m))) {
			first = append(first, value)
		} else {
			rest = append(rest, value)
		}
	}
	directUsers := []string{}
	for _, cl := range clients {
		if cl.Effective == "DIRECT" && (!p.Harden && !p.AdsOnly || cl.Class == "DIRECT") {
			directUsers = append(directUsers, cl.Email)
		}
	}
	sort.Strings(directUsers)
	if p.Harden {
		// The bounded TCP resolver pool is direct in category-only mode.
		next["dns"] = managedDNS()
		dnsDestination := destination
		if p.AdsOnly || !p.Residential && p.Direct {
			dnsDestination = directTag
		}
		if !p.Residential && !p.Direct {
			dnsDestination = blockedTag
		}
		if len(p.Proxies) > 0 && !proxyHasUDP(p.Proxies[0]) && dnsDestination == p.Proxies[0].Tag {
			first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-dns-udp", "inboundTag": []string{dnsTag}, "network": "udp", "outboundTag": blockedTag})
		}
		dnsRule := map[string]any{"type": "field", "ruleTag": "dob-route-dns", "inboundTag": []string{dnsTag}, "network": "tcp,udp", "outboundTag": dnsDestination}
		if p.StrictAllowlist {
			restrictInternalDNS(dnsRule, p)
		}
		first = append(first, dnsRule)
	}
	if len(tags) > 0 {
		if len(directUsers) > 0 {
			first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-direct-users", "inboundTag": tags, "user": directUsers, "network": "tcp,udp", "outboundTag": directTag})
		}
		if !p.Residential && p.Direct && !p.StrictAllowlist {
			destination = directTag
		}
		if !p.Residential && !p.Direct {
			destination = blockedTag
		}
		if !p.AdsOnly {
			// HTTP proxying cannot carry UDP. Deny it explicitly instead of allowing an
			// unmatched UDP packet to use the original first (direct) outbound.
			if len(p.Proxies) > 0 && !proxyHasUDP(p.Proxies[0]) && destination == p.Proxies[0].Tag {
				first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-residential-udp", "inboundTag": tags, "network": "udp", "outboundTag": blockedTag})
			}
			first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-default", "inboundTag": tags, "network": "tcp,udp", "outboundTag": destination})
		} else {
			if p.StrictAllowlist && !p.LegacyClientPaths {
				first = append(first, clientInfrastructureRules(tags, p)...)
			}
			if p.Harden && (p.Residential || p.StrictAllowlist) {
				dnsDestination := clientDNSTag
				if p.SniffingBlocked || p.StrictAllowlist {
					dnsDestination = blockedTag
				}
				first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-client-dns", "inboundTag": tags, "port": "53", "network": "tcp,udp", "outboundTag": dnsDestination})
			}
			if p.Residential {
				// HTTP cannot carry UDP: block only matched protected domains.
				// Never send matched Ads UDP to the non-ad direct fallback.
				if len(p.Proxies) > 0 && !proxyHasUDP(p.Proxies[0]) {
					first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-residential-udp", "inboundTag": tags, "domain": p.residentialDomains(), "network": "udp", "outboundTag": blockedTag})
				}
				first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-residential-ads", "inboundTag": tags, "domain": p.residentialDomains(), "network": "tcp,udp", "outboundTag": destination})
				if p.StrictAllowlist && p.LegacyClientPaths {
					first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-residential-probes", "inboundTag": tags, "domain": legacyProbeDomains(), "network": "tcp", "port": "80,443", "outboundTag": destination})
				}
			}
			fallback := directTag
			if p.StrictAllowlist || (!p.Residential && !p.Direct) || (p.Residential && p.SniffingBlocked) {
				fallback = blockedTag
			}
			// Strict mode denies all unmatched requests, including opaque/IP traffic.
			// Legacy selective mode is retained only for scoped rollback.
			first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-default", "inboundTag": tags, "network": "tcp,udp", "outboundTag": fallback})

		}

	}
	if err := configureRelayInlet(next, current, p, destination, &first); err != nil {
		return nil, err
	}
	if p.Harden {
		// Newly added/unobserved inbounds must not inherit the original direct
		// default during the interval before inventory reconciliation.
		first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-unobserved-inbound", "network": "tcp,udp", "outboundTag": blockedTag})
		for i, v := range kept {
			if v.(map[string]any)["tag"] == blockedTag {
				kept = append([]any{v}, append(kept[:i], kept[i+1:]...)...)
				break
			}
		}
		next["outbounds"] = kept
	}
	routing["rules"] = append(first, rest...)
	next["routing"] = routing
	if !p.PoolEnabled {
		removeManagedPool(next)
	}
	if p.PoolEnabled {
		if err := configurePool(next, p); err != nil {
			return nil, err
		}
		kept = next["outbounds"].([]any)
	}
	if err := residentialperf.Apply(next, p.Performance); err != nil {
		return nil, err
	}
	// Fingerprint effective routes and credentials. Direct membership is already
	// encoded in next; residential-only turnover does not change routing.
	// Runtime TestRoute returns this content-addressed tag only after the plan loads.
	fingerprintInput := map[string]any{"settings": next, "tags": tags}
	if !p.StableFingerprint {
		fingerprintInput["clients"] = clients // Exact legacy rollback path.
	}
	fingerprint := settingsHash(fingerprintInput)[:16]
	replacements := map[string]string{}
	for _, value := range kept {
		m := value.(map[string]any)
		tag, _ := m["tag"].(string)
		if managedTag(tag) {
			replacements[tag] = tag + "-" + fingerprint
			m["tag"] = replacements[tag]
		}
	}
	for _, value := range kept {
		m := value.(map[string]any)
		if stream, ok := m["streamSettings"].(map[string]any); ok {
			if sockopt, ok := stream["sockopt"].(map[string]any); ok {
				tag, _ := sockopt["dialerProxy"].(string)
				if replacement, ok := replacements[tag]; ok {
					sockopt["dialerProxy"] = replacement
				}
			}
		}
	}
	for _, value := range routing["rules"].([]any) {
		m := value.(map[string]any)
		tag, _ := m["outboundTag"].(string)
		if replacement, ok := replacements[tag]; ok {
			m["outboundTag"] = replacement
		}
	}
	if p.PoolEnabled {
		fingerprintPool(next, replacements)
	}
	return next, nil
}

// Preserve destination-IP deny rules ahead of managed routing, including mixed
// private CIDRs. Domain/ad and protocol-only rules remain for other inbounds.
func destinationIPGuard(rule map[string]any) bool {
	value, ok := rule["ip"]
	return ok && value != nil
}
