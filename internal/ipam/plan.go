package ipam

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

type Candidate struct {
	AccountID            string     `json:"account_id"`
	ServerID             string     `json:"server_id"`
	Region               string     `json:"region"`
	Healthy              bool       `json:"healthy"`
	Supported            bool       `json:"supported"`
	CapabilityAt         time.Time  `json:"capability_at"`
	CredentialGeneration string     `json:"credential_generation"`
	Family               int        `json:"family"`
	Product              string     `json:"product"`
	CapabilityKnown      bool       `json:"capability_known"`
	Slots                *int       `json:"slots"`
	Existing             int        `json:"existing"`
	Load                 int        `json:"load"`
	ExpiresAt            *time.Time `json:"expires_at"`
	NetworkImpact        string     `json:"network_impact"`
}
type Quote struct {
	Currency             string    `json:"currency"`
	HourlyMicros         *int64    `json:"hourly_micros"`
	DetachedHourlyMicros *int64    `json:"detached_hourly_micros"`
	GranularitySeconds   int       `json:"granularity_seconds"`
	Source               string    `json:"source"`
	ExpiresAt            time.Time `json:"expires_at"`
}
type PlanInput struct {
	AccountID                string      `json:"account_id"`
	PolicyRevision           int64       `json:"policy_revision"`
	Policy                   Policy      `json:"policy"`
	Quota                    *int        `json:"quota"`
	TotalUsed                int         `json:"total_used"`
	ManagedActive            int         `json:"managed_active"`
	Reserved                 int         `json:"reserved"`
	PendingRelease           int         `json:"pending_release"`
	UnknownOperations        int         `json:"unknown_operations"`
	ExistingCommitmentMicros int64       `json:"existing_commitment_micros"`
	InventoryAt              time.Time   `json:"inventory_at"`
	CredentialGeneration     string      `json:"credential_generation"`
	Quote                    Quote       `json:"quote"`
	Candidates               []Candidate `json:"candidates"`
}
type Assignment struct {
	ServerID      string `json:"server_id"`
	Region        string `json:"region"`
	NetworkImpact string `json:"network_impact"`
}
type Plan struct {
	State                     string       `json:"state"`
	Blockers                  []string     `json:"blockers"`
	Assignments               []Assignment `json:"assignments"`
	EstimatedCommitmentMicros *int64       `json:"estimated_commitment_micros"`
	Currency                  string       `json:"currency"`
	Digest                    string       `json:"digest"`
	Input                     PlanInput    `json:"input"`
	CreatedAt                 time.Time    `json:"created_at"`
}

func Preview(in PlanInput, now time.Time) Plan {
	p := Plan{State: "BLOCKED", Blockers: []string{}, Assignments: []Assignment{}, Input: in, CreatedAt: now, Currency: in.Policy.Currency}
	block := func(s string) { p.Blockers = append(p.Blockers, s) }
	if e := in.Policy.Validate(); e != nil {
		block("POLICY_INCOMPLETE")
	}
	if !in.Policy.Enabled {
		block("POLICY_DISABLED")
	}
	if in.AccountID == "" || in.PolicyRevision < 1 || in.CredentialGeneration == "" {
		block("ACCOUNT_EVIDENCE_MISSING")
	}
	if in.InventoryAt.IsZero() || in.InventoryAt.After(now) || now.Sub(in.InventoryAt) > 5*time.Minute {
		block("INVENTORY_STALE")
	}
	if in.Quota == nil {
		block("QUOTA_UNKNOWN")
	} else if *in.Quota < 0 {
		block("QUOTA_INVALID")
	}
	if in.TotalUsed < 0 || in.TotalUsed > 1000000 || in.ManagedActive < 0 || in.ManagedActive > in.TotalUsed || in.Reserved < 0 || in.Reserved > 10000 || in.PendingRelease < 0 || in.PendingRelease > in.TotalUsed || in.UnknownOperations < 0 || in.ExistingCommitmentMicros < 0 || in.ExistingCommitmentMicros > 1000000000000 {
		block("INVENTORY_INVALID")
	}
	seenServers := map[string]bool{}
	for _, c := range in.Candidates {
		if seenServers[c.ServerID] || c.Existing < 0 || c.Existing > 10000 || c.Load < 0 || (c.Slots != nil && (*c.Slots < 0 || *c.Slots > 10000)) {
			block("INVENTORY_INVALID")
			break
		}
		seenServers[c.ServerID] = true
	}
	if in.UnknownOperations > 0 {
		block("UNKNOWN_OPERATION")
	}
	if in.Quote.HourlyMicros == nil || in.Quote.DetachedHourlyMicros == nil || in.Quote.Source == "" || !in.Quote.ExpiresAt.After(now) || in.Quote.Currency != in.Policy.Currency || in.Quote.GranularitySeconds < 1 || in.Quote.GranularitySeconds > 86400 {
		block("COST_UNKNOWN_OR_EXPIRED")
	}
	if len(p.Blockers) == 0 {
		hourly := max(*in.Quote.HourlyMicros, *in.Quote.DetachedHourlyMicros)
		if *in.Quote.HourlyMicros < 0 || *in.Quote.DetachedHourlyMicros < 0 || hourly > 1000000000 {
			block("COST_INVALID")
		} else {
			horizon := int64(*in.Policy.FundedHorizonSeconds)
			g := int64(in.Quote.GranularitySeconds)
			billed := ((horizon + g - 1) / g) * g
			unit := (hourly*billed + 3599) / 3600
			n := min(*in.Policy.Desired-in.ManagedActive-in.Reserved, *in.Policy.PerRun, *in.Policy.MaxConcurrent-in.Reserved)
			available := *in.Quota - in.TotalUsed - in.Reserved // pending releases remain included in TotalUsed.
			if n > available {
				n = available
			}
			if n <= 0 {
				block("NO_SPARE_QUOTA_OR_TARGET_SATISFIED")
			} else {
				cs := append([]Candidate(nil), in.Candidates...)
				for i := 0; i < n; i++ {
					sort.Slice(cs, func(i, j int) bool {
						if cs[i].Existing != cs[j].Existing {
							return cs[i].Existing < cs[j].Existing
						}
						if cs[i].Load != cs[j].Load {
							return cs[i].Load < cs[j].Load
						}
						return cs[i].ServerID < cs[j].ServerID
					})
					selected := -1
					for j, c := range cs {
						if c.AccountID == in.AccountID && c.ServerID != "" && c.Region != "" && c.Healthy && c.CapabilityKnown && c.Supported && c.Family == in.Policy.Family && c.Product == in.Policy.Product && c.CredentialGeneration == in.CredentialGeneration && !c.CapabilityAt.After(now) && now.Sub(c.CapabilityAt) <= 5*time.Minute && c.Slots != nil && *c.Slots > c.Existing && c.NetworkImpact != "" && (c.ExpiresAt == nil || c.ExpiresAt.After(now.Add(time.Duration(*in.Policy.LifetimeMaxSeconds)*time.Second))) {
							selected = j
							break
						}
					}
					if selected < 0 {
						block("NO_ELIGIBLE_SERVER")
						break
					}
					c := &cs[selected]
					p.Assignments = append(p.Assignments, Assignment{c.ServerID, c.Region, c.NetworkImpact})
					c.Existing++
				}
				total := int64(len(p.Assignments))*unit + in.ExistingCommitmentMicros
				p.EstimatedCommitmentMicros = &total
				if total > *in.Policy.BudgetMicros {
					block("BUDGET_INSUFFICIENT")
				}
			}
		}
	}
	if len(p.Blockers) == 0 {
		p.State = "AWAITING_APPROVAL"
	}
	raw, _ := json.Marshal(struct {
		Input       PlanInput
		Assignments []Assignment
		CreatedAt   time.Time
	}{in, p.Assignments, now})
	hash := sha256.Sum256(raw)
	p.Digest = hex.EncodeToString(hash[:])
	return p
}
