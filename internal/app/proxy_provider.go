package app

import "strings"

type ProxyCapabilities struct{ CountryTargeting, StickySession, SessionTTL bool }
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

type suffixSessionAdapter struct{}

func (suffixSessionAdapter) Name() string { return "suffix-session" }
func (suffixSessionAdapter) Match(base string) bool {
	return strings.Contains(base, "__") || strings.Contains(base, "sid.") || strings.Contains(base, "cr.")
}
func (suffixSessionAdapter) Capabilities() ProxyCapabilities {
	return ProxyCapabilities{CountryTargeting: true, StickySession: true, SessionTTL: true}
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
		p = append(p, "sid."+r.SessionID)
	}
	if len(p) == 0 {
		return base
	}
	return base + "__" + strings.Join(p, ";")
}

func proxyAdapterByName(name string) ProxyProviderAdapter {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "suffix-session":
		return suffixSessionAdapter{}
	default:
		return genericProxyAdapter{}
	}
}
func proxySessionUsername(adapter, base, country, session string, targeted bool) string {
	return proxyAdapterByName(adapter).Username(ProxySessionRequest{BaseUsername: base, CountryCode: country, SessionID: session, TargetCountry: targeted})
}
