package ipam

import (
	"strings"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }
func policy() Policy {
	return Policy{Enabled: true, Family: 6, Product: "routed", Desired: ptr(3), PerRun: ptr(3), MaxConcurrent: ptr(3), SpacingMinSeconds: ptr(60), SpacingMaxSeconds: ptr(120), LifetimeMinSeconds: ptr(3600), LifetimeMaxSeconds: ptr(7200), DiversityBits: ptr(64), AllocationBits: ptr(56), ActiveScope: "GLOBAL", HistoryScope: "GLOBAL", HistorySeconds: ptr(86400), Currency: "USD", BudgetMicros: ptr(int64(1000000)), FundedHorizonSeconds: ptr(86400)}
}
func TestNoImplicitPolicyOrPrefix(t *testing.T) {
	p := Policy{Family: 6}
	if p.Validate() != nil {
		t.Fatal("disabled policy rejected")
	}
	p.Enabled = true
	if p.Validate() == nil {
		t.Fatal("implicit defaults allowed")
	}
	p = policy()
	p.DiversityBits = nil
	if p.Validate() == nil {
		t.Fatal("implicit /64")
	}
}
func TestAddressAndPrefixDiversity(t *testing.T) {
	a, e := NormalizeAddress(" 2001:0db8:0001:0000::0100 ")
	if e != nil || a.String() != "2001:db8:1::100" {
		t.Fatal(a, e)
	}
	b, e := NormalizeAddress("::ffff:203.0.113.8")
	if e != nil || b.String() != "203.0.113.8" {
		t.Fatal(b, e)
	}
	x, _ := DiversityPrefix("2001:db8:1::100", "2001:db8:1::/56", 64)
	y, _ := DiversityPrefix("2001:db8:1::101", "2001:db8:1::/56", 64)
	z, _ := DiversityPrefix("2001:db8:1:1::100", "2001:db8:1::/56", 64)
	if !PrefixConflict(x, y) || PrefixConflict(x, z) {
		t.Fatal("host suffix confused with diversity", x, y, z)
	}
	broad, _ := DiversityPrefix("2001:db8:1:1::100", "2001:db8:1::/56", 56)
	if !PrefixConflict(broad, z) {
		t.Fatal("overlapping policy sizes bypass diversity")
	}
	if _, e = DiversityPrefix("2001:db8:2::1", "2001:db8:1::/56", 64); e == nil {
		t.Fatal("address outside allocation")
	}
	for _, s := range []string{"fe80::1%eth0", "127.0.0.1", "10.1.1.1", "::", "ff02::1"} {
		if _, e = NormalizeAddress(s); e == nil {
			t.Fatal("unsafe endpoint", s)
		}
	}
}
func TestEndpointURIChangesOnlyIngress(t *testing.T) {
	raw := "vless://client@203.0.113.1:443?security=reality&pbk=a%2Bb&sni=example.test#direct%20config"
	got, e := EndpointURI(raw, "2001:db8:1::1")
	if e != nil || got != "vless://client@[2001:db8:1::1]:443?security=reality&pbk=a%2Bb&sni=example.test#direct%20config" {
		t.Fatal(got, e)
	}
	got, e = EndpointURI(got, "203.0.113.9")
	if e != nil || !strings.Contains(got, "@203.0.113.9:443?") {
		t.Fatal(got, e)
	}
	for _, r := range []string{"http://a:443", "vless://a:443", "vless://c@a", "vless://c@a:0"} {
		if _, e = EndpointURI(r, "203.0.113.2"); e == nil {
			t.Fatal(r)
		}
	}
}
func TestRetirementDoesNotConfuseVisibilityAndLease(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Second)
	future := now.Add(time.Hour)
	for _, ls := range [][]Lease{{{Until: nil}}, {{Until: &future}}} {
		if RetirementAllowed(true, false, true, ls, now) == nil {
			t.Fatal("live or unlimited lease retired")
		}
	}
	if RetirementAllowed(true, false, true, []Lease{{Until: &past}, {Migrated: true}}, now) != nil {
		t.Fatal("expired/migrated lease blocked")
	}
	if RetirementAllowed(false, false, true, nil, now) == nil || RetirementAllowed(true, true, true, nil, now) == nil || RetirementAllowed(true, false, false, nil, now) == nil {
		t.Fatal("unsafe release")
	}
}
func TestSelectedLifetimeWithinBounds(t *testing.T) {
	for i := 0; i < 50; i++ {
		n, e := ChooseSeconds(10, 20)
		if e != nil || n < 10 || n > 20 {
			t.Fatal(n, e)
		}
	}
	if n, e := ChooseSeconds(10, 10); e != nil || n != 10 {
		t.Fatal(n, e)
	}
}

func TestAllocationPrefixMustBeExplicitAndFamilyCompatible(t *testing.T) {
	p := policy()
	p.AllocationBits = nil
	if p.Validate() == nil {
		t.Fatal("implicit allocation prefix")
	}
	p = policy()
	p.Family = 4
	p.AllocationBits = ptr(33)
	if p.Validate() == nil {
		t.Fatal("IPv6 allocation length accepted for IPv4")
	}
}
