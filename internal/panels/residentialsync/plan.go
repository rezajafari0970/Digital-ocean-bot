package residentialsync

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const directTag = "dob-route-direct"
const blockedTag = "dob-route-blocked"

// Installed geosite.dat contains these lists. "Google ads" and "Ads" are
// display names, not valid geosite:google-ads / geosite:ads identifiers.
var adDomains = []string{"geosite:category-ads-all", "geosite:category-ads", "geosite:google@ads", "geosite:facebook@ads"}

type clientRoute struct{ ID, Email, Class, Effective string }
type routePolicy struct {
	Explicit            bool
	Residential, Direct bool
	AdsOnly             bool
	Harden              bool
	SniffingBlocked     bool
	Configured          int
	HealthyCount        int
	Proxies             []rp
}
type rp struct {
	ID, Type, Host, User, Tag, Password string
	Port                                int
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
			} else if p.Configured == 0 && !p.Harden {
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
	if p.Configured == 0 && !p.Harden {
		destination = directTag
	} else if len(p.Proxies) > 0 {
		proxy := p.Proxies[0]
		protocol := "http"
		if proxy.Type == "socks5" {
			protocol = "socks"
		} else if proxy.Type != "http" && proxy.Type != "https" {
			return nil, errors.New("unsupported residential protocol")
		}
		server := map[string]any{"address": proxy.Host, "port": proxy.Port}
		if proxy.User != "" || proxy.Password != "" {
			server["users"] = []any{map[string]any{"user": proxy.User, "pass": proxy.Password}}
		}
		outbound := map[string]any{"tag": proxy.Tag, "protocol": protocol, "settings": map[string]any{"servers": []any{server}}}
		if proxy.Type == "https" {
			outbound["streamSettings"] = map[string]any{"security": "tls", "tlsSettings": map[string]any{"serverName": proxy.Host}}
		}
		kept = append(kept, outbound)
		destination = proxy.Tag
	}
	if p.SniffingBlocked {
		destination = blockedTag
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
		if cl.Effective == "DIRECT" && (!p.Harden || cl.Class == "DIRECT") {
			directUsers = append(directUsers, cl.Email)
		}
	}
	sort.Strings(directUsers)
	if p.Harden {
		// Built-in DNS never uses localhost or a direct local-mode transport.
		next["dns"] = managedDNS()
		dnsDestination := destination
		if !p.Residential && p.Direct {
			dnsDestination = directTag
		}
		if !p.Residential && !p.Direct {
			dnsDestination = blockedTag
		}
		if len(p.Proxies) > 0 && p.Proxies[0].Type != "socks5" && dnsDestination == p.Proxies[0].Tag {
			first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-dns-udp", "inboundTag": []string{dnsTag}, "network": "udp", "outboundTag": blockedTag})
		}
		first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-dns", "inboundTag": []string{dnsTag}, "network": "tcp,udp", "outboundTag": dnsDestination})
	}
	if len(tags) > 0 {
		if len(directUsers) > 0 {
			first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-direct-users", "inboundTag": tags, "user": directUsers, "network": "tcp,udp", "outboundTag": directTag})
		}
		if !p.Residential && p.Direct {
			destination = directTag
		}
		if !p.Residential && !p.Direct {
			destination = blockedTag
		}
		if !p.AdsOnly {
			// HTTP proxying cannot carry UDP. Deny it explicitly instead of allowing an
			// unmatched UDP packet to use the original first (direct) outbound.
			if len(p.Proxies) > 0 && p.Proxies[0].Type != "socks5" && destination == p.Proxies[0].Tag {
				first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-residential-udp", "inboundTag": tags, "network": "udp", "outboundTag": blockedTag})
			}
			first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-default", "inboundTag": tags, "network": "tcp,udp", "outboundTag": destination})
		} else {
			// Ad rules precede the known-domain direct rule. Opaque/IP traffic and
			// DNS queries cannot prove that they are non-advertising: fail closed.
			// A domain outside the geosite lists can still contain ads; sniffing is
			// not a guarantee for ECH, misleading hostnames, or incomplete lists.
			if p.Harden && p.Residential {
				if len(p.Proxies) > 0 && p.Proxies[0].Type != "socks5" && destination == p.Proxies[0].Tag {
					first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-client-dns-udp", "inboundTag": tags, "port": "53", "network": "udp", "outboundTag": blockedTag})
				}
				first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-client-dns", "inboundTag": tags, "port": "53", "network": "tcp,udp", "outboundTag": destination})
			}
			if p.Residential {
				if len(p.Proxies) > 0 && p.Proxies[0].Type != "socks5" && destination == p.Proxies[0].Tag {
					first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-residential-udp", "inboundTag": tags, "domain": adDomains, "network": "udp", "outboundTag": blockedTag})
				}
				first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-residential-ads", "inboundTag": tags, "domain": adDomains, "network": "tcp,udp", "outboundTag": destination})
			}
			fallback := directTag
			if p.Harden && p.Residential {
				if !p.SniffingBlocked {
					first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-known-non-ad", "inboundTag": tags, "domain": []string{knownDomainPattern}, "network": "tcp,udp", "outboundTag": directTag})
				}
				fallback = destination
				if len(p.Proxies) > 0 && p.Proxies[0].Type != "socks5" && destination == p.Proxies[0].Tag {
					first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-opaque-udp", "inboundTag": tags, "network": "udp", "outboundTag": blockedTag})
				}
			}
			if !p.Residential && !p.Direct {
				fallback = blockedTag
			}
			first = append(first, map[string]any{"type": "field", "ruleTag": "dob-route-default", "inboundTag": tags, "network": "tcp,udp", "outboundTag": fallback})

		}

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
	// Fingerprint the entire desired plan, including credentials and client membership.
	// Runtime TestRoute returns this content-addressed tag only after the plan loads.
	fingerprint := settingsHash(map[string]any{"settings": next, "clients": clients, "tags": tags})[:16]
	replacements := map[string]string{}
	for _, value := range kept {
		m := value.(map[string]any)
		tag, _ := m["tag"].(string)
		if managedTag(tag) {
			replacements[tag] = tag + "-" + fingerprint
			m["tag"] = replacements[tag]
		}
	}
	for _, value := range routing["rules"].([]any) {
		m := value.(map[string]any)
		tag, _ := m["outboundTag"].(string)
		if replacement, ok := replacements[tag]; ok {
			m["outboundTag"] = replacement
		}
	}
	return next, nil
}

// Preserve destination-IP deny rules ahead of managed routing, including mixed
// private CIDRs. Domain/ad and protocol-only rules remain for other inbounds.
func destinationIPGuard(rule map[string]any) bool {
	value, ok := rule["ip"]
	return ok && value != nil
}
