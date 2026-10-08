package residentialsync

import (
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/resolverpolicy"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"os"
	"strings"
)

// Exact hosts used by client URL tests and the residential pool observer.
// HTTPS paths cannot be inspected by Xray routing; this is host+port policy.
var residentialProbeDomains = []string{
	"full:www.gstatic.com",
	"full:connectivitycheck.gstatic.com",
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
