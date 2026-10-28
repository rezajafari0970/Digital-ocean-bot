package app

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type ProxyCapabilities struct {
	CountryTargeting, StickySession, SessionTTL bool
	// These are explicit driver contracts, never inferred from one geo sample.
	FixedEgress, DedicatedPort bool
}
type ProxySessionRequest struct {
	BaseUsername, CountryCode, SessionID string
	TargetCountry                        bool
}
type ProxyProviderAdapter interface {
	Name() string
	Match(string) bool
	Capabilities() ProxyCapabilities
	Username(ProxySessionRequest) string
}

type genericProxyAdapter struct{}

func (genericProxyAdapter) Name() string                          { return "generic" }
func (genericProxyAdapter) Match(string) bool                     { return true }
func (genericProxyAdapter) Capabilities() ProxyCapabilities       { return ProxyCapabilities{} }
func (genericProxyAdapter) Username(r ProxySessionRequest) string { return r.BaseUsername }

// Fixed proxies use their configured endpoint/credentials verbatim. Country
// eligibility still requires live country and reconnect verification.
type fixedProxyAdapter struct{ genericProxyAdapter }

func (fixedProxyAdapter) Name() string { return "fixed" }
func (fixedProxyAdapter) Capabilities() ProxyCapabilities {
	return ProxyCapabilities{FixedEgress: true}
}

type suffixSessionAdapter struct{}

func (suffixSessionAdapter) Name() string { return "suffix-session" }
func (suffixSessionAdapter) Match(base string) bool {
	return strings.Contains(base, "__") || strings.Contains(base, "sid.") || strings.Contains(base, "cr.")
}
func (suffixSessionAdapter) Capabilities() ProxyCapabilities {
	return ProxyCapabilities{CountryTargeting: true, StickySession: true, SessionTTL: true, DedicatedPort: true}
}
func (suffixSessionAdapter) Username(r ProxySessionRequest) string {
	base := r.BaseUsername
	if i := strings.Index(base, "__"); i >= 0 {
		base = base[:i]
	}
	p := []string{}
	if r.TargetCountry && r.CountryCode != "" {
		p = append(p, "cr."+strings.ToLower(r.CountryCode))
	}
	if r.SessionID != "" {
		p = append(p, "sid."+r.SessionID, "sessttl.120")
	}
	if len(p) == 0 {
		return base
	}
	return base + "__" + strings.Join(p, ";")
}

var proxyDrivers = struct {
	sync.RWMutex
	adapters map[string]ProxyProviderAdapter
}{adapters: map[string]ProxyProviderAdapter{"generic": genericProxyAdapter{}, "fixed": fixedProxyAdapter{}, "suffix-session": suffixSessionAdapter{}}}

// RegisterProxyAdapter is the plugin boundary. Applications register drivers
// during startup; replacing an already registered credential grammar is refused.
func RegisterProxyAdapter(adapter ProxyProviderAdapter) error {
	if adapter == nil {
		return errors.New("missing proxy driver")
	}
	name := strings.ToLower(strings.TrimSpace(adapter.Name()))
	if !regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`).MatchString(name) || name != adapter.Name() {
		return errors.New("invalid proxy driver name")
	}
	proxyDrivers.Lock()
	defer proxyDrivers.Unlock()
	if _, exists := proxyDrivers.adapters[name]; exists {
		return errors.New("proxy driver already registered")
	}
	proxyDrivers.adapters[name] = adapter
	return nil
}

func ProxyAdapterRegistered(name string) bool {
	if name == "" {
		name = "generic"
	}
	proxyDrivers.RLock()
	defer proxyDrivers.RUnlock()
	_, ok := proxyDrivers.adapters[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func ProxyAdapterNames() []string {
	proxyDrivers.RLock()
	defer proxyDrivers.RUnlock()
	names := make([]string, 0, len(proxyDrivers.adapters))
	for name := range proxyDrivers.adapters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func proxyAdapterByName(name string) ProxyProviderAdapter {
	proxyDrivers.RLock()
	defer proxyDrivers.RUnlock()
	if adapter := proxyDrivers.adapters[strings.ToLower(strings.TrimSpace(name))]; adapter != nil {
		return adapter
	}
	return genericProxyAdapter{}
}
func proxySessionUsername(adapter, base, country, session string, targeted bool) string {
	driver := proxyAdapterByName(adapter)
	targeted = targeted && driver.Capabilities().CountryTargeting
	return driver.Username(ProxySessionRequest{BaseUsername: base, CountryCode: country, SessionID: session, TargetCountry: targeted})
}

// ProxySessionUsername exposes the canonical adapter rewrite for preview/validation paths.
func ProxySessionUsername(adapter, base, country, session string, targeted bool) string {
	return proxySessionUsername(adapter, base, country, session, targeted)
}

func ProxyAdapterCapabilities(adapter string) ProxyCapabilities {
	return proxyAdapterByName(adapter).Capabilities()
}
