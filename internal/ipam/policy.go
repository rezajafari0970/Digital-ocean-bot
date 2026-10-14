// Package ipam manages additional IP plans independently from compute scheduling.
// No policy or plan grants authority to mutate a provider or guest network.
package ipam

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrPolicy = errors.New("IP policy incomplete or invalid")
var ErrProtected = errors.New("IP resource has protected or unresolved obligations")

type Policy struct {
	Enabled              bool   `json:"enabled"`
	Family               int    `json:"family"`
	Product              string `json:"product"`
	Desired              *int   `json:"desired"`
	PerRun               *int   `json:"per_run"`
	MaxConcurrent        *int   `json:"max_concurrent"`
	SpacingMinSeconds    *int   `json:"spacing_min_seconds"`
	SpacingMaxSeconds    *int   `json:"spacing_max_seconds"`
	LifetimeMinSeconds   *int   `json:"lifetime_min_seconds"`
	LifetimeMaxSeconds   *int   `json:"lifetime_max_seconds"`
	DiversityBits        *int   `json:"diversity_bits"`
	AllocationBits       *int   `json:"allocation_bits"`
	ActiveScope          string `json:"active_scope"`
	HistoryScope         string `json:"history_scope"`
	HistorySeconds       *int   `json:"history_seconds"`
	Currency             string `json:"currency"`
	BudgetMicros         *int64 `json:"budget_micros"`
	FundedHorizonSeconds *int   `json:"funded_horizon_seconds"`
}

func (p Policy) Validate() error {
	if p.Family != 4 && p.Family != 6 {
		return ErrPolicy
	}
	if !p.Enabled {
		return nil
	}
	if p.Product == "" || p.Desired == nil || *p.Desired < 0 || *p.Desired > 10000 || p.PerRun == nil || *p.PerRun < 1 || *p.PerRun > 100 || p.MaxConcurrent == nil || *p.MaxConcurrent < 1 || *p.MaxConcurrent > 8 {
		return ErrPolicy
	}
	for _, pair := range [][2]*int{{p.SpacingMinSeconds, p.SpacingMaxSeconds}, {p.LifetimeMinSeconds, p.LifetimeMaxSeconds}} {
		if pair[0] == nil || pair[1] == nil || *pair[0] < 1 || *pair[0] > *pair[1] || *pair[1] > 31536000 {
			return ErrPolicy
		}
	}
	if p.ActiveScope != "GLOBAL" && p.ActiveScope != "ACCOUNT" {
		return ErrPolicy
	}
	if p.HistoryScope != "GLOBAL" && p.HistoryScope != "ACCOUNT" {
		return ErrPolicy
	}
	if p.HistorySeconds == nil || *p.HistorySeconds < 0 || *p.HistorySeconds > 315360000 {
		return ErrPolicy
	}
	if p.Family == 6 && (p.DiversityBits == nil || *p.DiversityBits < 1 || *p.DiversityBits > 128) {
		return ErrPolicy
	}
	if (p.Family == 6 && p.AllocationBits == nil) || (p.AllocationBits != nil && (*p.AllocationBits < 1 || *p.AllocationBits > 128 || (p.Family == 4 && *p.AllocationBits > 32))) {
		return ErrPolicy
	}
	if p.Currency != "USD" && p.Currency != "EUR" {
		return ErrPolicy
	}
	if p.BudgetMicros == nil || *p.BudgetMicros < 0 || *p.BudgetMicros > 1000000000000 || p.FundedHorizonSeconds == nil || *p.FundedHorizonSeconds < *p.LifetimeMaxSeconds || *p.FundedHorizonSeconds > 31536000 {
		return ErrPolicy
	}
	return nil
}

// NormalizeAddress is syntactic canonicalization with basic local-address rejection.
// It is not proof of global routability, provider ownership or publication eligibility.
func NormalizeAddress(raw string) (netip.Addr, error) {
	a, e := netip.ParseAddr(strings.TrimSpace(raw))
	if e != nil || a.Zone() != "" {
		return netip.Addr{}, ErrPolicy
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return netip.Addr{}, ErrPolicy
	}
	return a, nil
}
func DiversityPrefix(address, parent string, bits int) (netip.Prefix, error) {
	a, e := NormalizeAddress(address)
	if e != nil || !a.Is6() || bits < 1 || bits > 128 {
		return netip.Prefix{}, ErrPolicy
	}
	p, e := netip.ParsePrefix(parent)
	if e != nil || !p.Addr().Is6() || p.Addr().Is4In6() || !p.Contains(a) {
		return netip.Prefix{}, ErrPolicy
	}
	// A broader diversity unit is valid but consumes the entire broader unit.
	d, e := a.Prefix(bits)
	if e != nil {
		return netip.Prefix{}, e
	}
	return d.Masked(), nil
}
func PrefixConflict(a, b netip.Prefix) bool { return a.IsValid() && b.IsValid() && a.Overlaps(b) }
func ChooseSeconds(min, max int) (int, error) {
	if min < 1 || max < min || max > 31536000 {
		return 0, ErrPolicy
	}
	n, e := rand.Int(rand.Reader, big.NewInt(int64(max-min)+1))
	if e != nil {
		return 0, e
	}
	return min + int(n.Int64()), nil
}
func EndpointURI(raw, address string) (string, error) {
	a, e := NormalizeAddress(address)
	if e != nil {
		return "", e
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "vless" || u.User == nil || u.User.Username() == "" || u.Hostname() == "" {
		return "", ErrPolicy
	}
	n, e := strconv.Atoi(u.Port())
	if e != nil || n < 1 || n > 65535 {
		return "", ErrPolicy
	}
	// Preserve original query and fragment bytes; only the authority host changes.
	start := strings.Index(raw, "://") + 3
	end := len(raw)
	for _, delim := range []string{"/", "?", "#"} {
		if i := strings.Index(raw[start:], delim); i >= 0 && start+i < end {
			end = start + i
		}
	}
	at := strings.LastIndex(raw[start:end], "@")
	if at < 0 {
		return "", ErrPolicy
	}
	host := a.String()
	if a.Is6() {
		host = "[" + host + "]"
	}
	return raw[:start+at+1] + host + ":" + u.Port() + raw[end:], nil
}

type Lease struct {
	Until    *time.Time
	Migrated bool
}

func RetirementAllowed(owned bool, unknown bool, replacementVerified bool, leases []Lease, now time.Time) error {
	if !owned || unknown || !replacementVerified {
		return ErrProtected
	}
	for _, l := range leases {
		if !l.Migrated && (l.Until == nil || l.Until.After(now)) {
			return ErrProtected
		}
	}
	return nil
}
func decodePolicy(raw []byte) (Policy, error) {
	var p Policy
	e := json.Unmarshal(raw, &p)
	if e != nil {
		return p, e
	}
	return p, p.Validate()
}
