package ipam

import (
	"slices"
	"testing"
	"time"
)

func input(now time.Time) PlanInput {
	return PlanInput{AccountID: "a", PolicyRevision: 1, Policy: policy(), Quota: ptr(20), TotalUsed: 10, ManagedActive: 0, InventoryAt: now, CredentialGeneration: "g1", Quote: Quote{Currency: "USD", HourlyMicros: ptr(int64(1000)), DetachedHourlyMicros: ptr(int64(10000)), GranularitySeconds: 3600, Source: "account quote", ExpiresAt: now.Add(time.Hour)}, Candidates: []Candidate{{AccountID: "a", ServerID: "s1", Region: "r", Healthy: true, Supported: true, CapabilityKnown: true, CapabilityAt: now, CredentialGeneration: "g1", Family: 6, Product: "routed", Slots: ptr(3), NetworkImpact: "manual network approval required"}, {AccountID: "a", ServerID: "s2", Region: "r", Healthy: true, Supported: true, CapabilityKnown: true, CapabilityAt: now, CredentialGeneration: "g1", Family: 6, Product: "routed", Slots: ptr(3), NetworkImpact: "manual network approval required"}}}
}
func TestPlanUsesManagedTargetAndTotalQuotaSeparately(t *testing.T) {
	now := time.Now()
	in := input(now)
	p := Preview(in, now)
	if p.State != "AWAITING_APPROVAL" || len(p.Assignments) != 3 {
		t.Fatal(p.State, p.Blockers, len(p.Assignments))
	}
	if p.Assignments[0].ServerID == p.Assignments[1].ServerID {
		t.Fatal("concentrated allocation")
	}
	if *p.EstimatedCommitmentMicros != 720000 {
		t.Fatal("detached/reserved cost omitted", *p.EstimatedCommitmentMicros)
	}
}
func TestPlanBlockers(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name, code string
		mutate     func(*PlanInput)
	}{
		{"quota", "QUOTA_UNKNOWN", func(p *PlanInput) { p.Quota = nil }},
		{"pending-release", "NO_SPARE_QUOTA_OR_TARGET_SATISFIED", func(p *PlanInput) { p.TotalUsed = 20; p.PendingRelease = 5 }},
		{"unknown", "UNKNOWN_OPERATION", func(p *PlanInput) { p.UnknownOperations = 1 }},
		{"stale", "INVENTORY_STALE", func(p *PlanInput) { p.InventoryAt = now.Add(-6 * time.Minute) }},
		{"quote", "COST_UNKNOWN_OR_EXPIRED", func(p *PlanInput) { p.Quote.DetachedHourlyMicros = nil }},
		{"budget", "BUDGET_INSUFFICIENT", func(p *PlanInput) { p.ExistingCommitmentMicros = 400000 }},
		{"account", "NO_ELIGIBLE_SERVER", func(p *PlanInput) {
			for i := range p.Candidates {
				p.Candidates[i].AccountID = "b"
			}
		}},
		{"generation", "NO_ELIGIBLE_SERVER", func(p *PlanInput) { p.CredentialGeneration = "g2" }},
		{"expired-capability", "NO_ELIGIBLE_SERVER", func(p *PlanInput) {
			for i := range p.Candidates {
				p.Candidates[i].CapabilityAt = now.Add(-time.Hour)
			}
		}},
		{"ipv6-unreachable", "NO_ELIGIBLE_SERVER", func(p *PlanInput) {
			for i := range p.Candidates {
				p.Candidates[i].Healthy = false
			}
		}},
		{"negative-price", "COST_INVALID", func(p *PlanInput) { p.Quote.HourlyMicros = ptr(int64(-1)) }},
		{"overflow", "INVENTORY_INVALID", func(p *PlanInput) { p.TotalUsed = int(^uint(0) >> 1) }},
		{"duplicate-server", "INVENTORY_INVALID", func(p *PlanInput) { p.Candidates[1] = p.Candidates[0] }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := input(now)
			c.mutate(&in)
			p := Preview(in, now)
			if p.State != "BLOCKED" || !slices.Contains(p.Blockers, c.code) {
				t.Fatal(p.State, p.Blockers)
			}
		})
	}
}
func TestApprovalDigestChangesWithEvidence(t *testing.T) {
	now := time.Now()
	a := input(now)
	b := input(now)
	b.PolicyRevision++
	if Preview(a, now).Digest == Preview(b, now).Digest {
		t.Fatal("policy change reused approval")
	}
	b = input(now)
	b.Candidates[0].NetworkImpact = "reboot"
	if Preview(a, now).Digest == Preview(b, now).Digest {
		t.Fatal("impact change reused approval")
	}
}
