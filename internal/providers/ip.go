package providers

import (
	"context"
	"fmt"
	"net/netip"
	"time"
)

// IP resources are not compute capacity. Nil quota means unknown, never unlimited.
type IPQuota struct {
	Family     int       `json:"family"`
	Kind       string    `json:"kind"`
	Limit      *int      `json:"limit"`
	Used       *int      `json:"used"`
	Scope      string    `json:"scope"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observed_at"`
}
type IPResource struct {
	ID       string `json:"id"`
	Family   int    `json:"family"`
	Kind     string `json:"kind"`
	Address  string `json:"address,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
	ServerID string `json:"server_id,omitempty"`
	Region   string `json:"region,omitempty"`
	// Discovery is never evidence of feature ownership or safe deletion.
	Protected bool `json:"protected"`
}
type IPObservation struct {
	CredentialGeneration string       `json:"credential_generation,omitempty"`
	Resources            []IPResource `json:"resources"`
	Quotas               []IPQuota    `json:"quotas"`
	Complete             bool         `json:"complete"`
	ObservedAt           time.Time    `json:"observed_at"`
}
type IPReader interface {
	ObserveIPs(context.Context) (IPObservation, error)
}

func IPDataError() error {
	return &Error{Class: ErrorUnavailable, Operation: "observe_ips", Code: "INVALID_IP_INVENTORY", Message: "incomplete or invalid IP inventory"}
}
func ValidateIPObservation(o IPObservation) error {
	if !o.Complete || o.ObservedAt.IsZero() || o.Resources == nil || o.Quotas == nil {
		return IPDataError()
	}
	seen := map[string]bool{}
	addresses := map[string]bool{}
	quotaKeys := map[string]bool{}
	for _, r := range o.Resources {
		if r.ID == "" || r.Kind == "" || (r.Family != 4 && r.Family != 6) {
			return IPDataError()
		}
		key := fmt.Sprintf("%d/%s/%s", r.Family, r.Kind, r.ID)
		if seen[key] {
			return IPDataError()
		}
		seen[key] = true
		if r.Address == "" && r.Prefix == "" {
			return IPDataError()
		}
		if r.Address != "" {
			a, e := netip.ParseAddr(r.Address)
			if e != nil || a.Zone() != "" || a.Is4In6() || a.Is4() != (r.Family == 4) {
				return IPDataError()
			}
		}
		if r.Address != "" {
			a, _ := netip.ParseAddr(r.Address)
			if addresses[a.String()] {
				return IPDataError()
			}
			addresses[a.String()] = true
		}
		if r.Prefix != "" {
			p, e := netip.ParsePrefix(r.Prefix)
			if e != nil || p.Addr().Is4In6() || p.Addr().Is4() != (r.Family == 4) || p != p.Masked() {
				return IPDataError()
			}
		}
	}
	for _, q := range o.Quotas {
		key := fmt.Sprintf("%d/%s/%s", q.Family, q.Kind, q.Scope)
		if quotaKeys[key] {
			return IPDataError()
		}
		quotaKeys[key] = true
		if (q.Family != 4 && q.Family != 6) || q.Kind == "" || q.Scope == "" || q.Source == "" || q.ObservedAt.IsZero() || (q.Limit != nil && *q.Limit < 0) || (q.Used != nil && *q.Used < 0) {
			return IPDataError()
		}
	}
	return nil
}
