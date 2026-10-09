package residentialsync

import (
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/resolverpolicy"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"os"
	"strings"
)

// Exact client URL-test hosts. Client probes use direct server egress; the
// independent pool observer still measures residential endpoints.
// HTTPS paths cannot be inspected by Xray routing; this is host+port policy.
var residentialProbeDomains = []string{
	"full:www.gstatic.com",
	"full:connectivitycheck.gstatic.com",
	"full:www.google.com",
}

func (p routePolicy) normalized() routePolicy {
	if p.StrictAllowlist {
		p.Harden = true
		p.AdsOnly = true
	}
	return p
}

// Scope is deployment-only: default/all covers existing and future panels.
// StrictAllowlistEnabled is shared with native acceptance so observed legacy
// settings cannot silently downgrade the expected deployment policy.
func StrictAllowlistEnabled(panel string) bool {
	scope := strings.TrimSpace(os.Getenv("DOB_RESIDENTIAL_ALLOWLIST_PANELS"))
	if scope == "" || scope == "all" {
		return true
	}
	if scope == "none" {
		return false
	}
	selected := false
	for _, item := range strings.Split(scope, ",") {
		id := strings.TrimSpace(item)
		// A malformed canary declaration fails strict for the entire scope.
		if !residentialperf.UUID.MatchString(id) {
			return true
		}
		selected = selected || strings.EqualFold(id, strings.TrimSpace(panel))
	}
	return selected
}
func strictAllowlistPanel(panel string) bool { return StrictAllowlistEnabled(panel) }

func restrictInternalDNS(rule map[string]any, p routePolicy) {
	rule["ip"] = resolverpolicy.Active()
	rule["port"] = internalDNSPort(p)
	rule["network"] = "tcp"
}

func internalDNSPort(p routePolicy) string {
	if p.Performance != nil && p.Performance.DNSMode == "doh" {
		return "443"
	}
	return "53"
}

// Internal DNS and loopback identities are infrastructure, never client inlets.
func validateStrictInboundTags(tags []string, current map[string]any) error {
	reserved := map[string]bool{"api": true}
	if api, ok := current["api"].(map[string]any); ok {
		if tag, ok := api["tag"].(string); ok {
			reserved[tag] = true
		}
	}
	if inbounds, ok := current["inbounds"].([]any); ok {
		for _, v := range inbounds {
			if in, ok := v.(map[string]any); ok {
				if tag, ok := in["tag"].(string); ok {
					reserved[tag] = true
				}
			}
		}
	}
	for _, tag := range tags {
		if strings.HasPrefix(tag, "dob-route-") || reserved[tag] {
			return errors.New("client inbound collides with reserved infrastructure routing identity")
		}
	}
	return nil
}

// The default v2rayNG encrypted resolver is an explicit infrastructure exception.
// TLS routing constrains this host/port, not encrypted paths or query names.
var clientDoHDomains = []string{"full:cloudflare-dns.com"}

func clientInfrastructureRules(tags []string, p routePolicy) []any {
	rules := []any{}
	if p.Residential && !p.SniffingBlocked {
		rules = append(rules,
			map[string]any{"type": "field", "ruleTag": "dob-route-client-probes", "inboundTag": tags, "domain": residentialProbeDomains, "network": "tcp", "port": "80,443", "outboundTag": directTag},
			map[string]any{"type": "field", "ruleTag": "dob-route-client-dns-allowed", "inboundTag": tags, "ip": resolverpolicy.Active(), "network": "tcp,udp", "port": "53", "outboundTag": clientDNSTag},
			map[string]any{"type": "field", "ruleTag": "dob-route-client-doh", "inboundTag": tags, "domain": clientDoHDomains, "network": "tcp", "port": "443", "outboundTag": directTag})
	}
	// Exact infrastructure hosts cannot fall through into a broader Ads geosite
	// entry on an unapproved port/transport, even if geodata changes later.
	rules = append(rules, map[string]any{"type": "field", "ruleTag": "dob-route-client-infrastructure-deny", "inboundTag": tags, "domain": append(append([]string{}, residentialProbeDomains...), clientDoHDomains...), "network": "tcp,udp", "outboundTag": blockedTag})
	return rules
}

// ClientPathsEnabled stages only the DNS/probe repair. Returning false keeps
// the pinned strict policy; it never falls back to unrestricted selective mode.
func ClientPathsEnabled(panel string) bool {
	scope := strings.TrimSpace(os.Getenv("DOB_RESIDENTIAL_CLIENT_PATHS_PANELS"))
	if scope == "" || scope == "all" {
		return true
	}
	if scope == "none" {
		return false
	}
	selected := false
	for _, item := range strings.Split(scope, ",") {
		id := strings.TrimSpace(item)
		if !residentialperf.UUID.MatchString(id) {
			return false
		}
		selected = selected || strings.EqualFold(id, strings.TrimSpace(panel))
	}
	return selected
}
func legacyProbeDomains() []string {
	return []string{"full:www.gstatic.com", "full:connectivitycheck.gstatic.com"}
}
