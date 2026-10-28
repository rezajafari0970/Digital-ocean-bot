package proxyselection

import (
	"context"
	"time"
)

type Health string

const (
	Usable   Health = "usable"
	Unusable Health = "unusable"
	Unknown  Health = "unknown"
)

// Provider is already restricted to this account's eligible assigned pool.
// Attempts is a driver's finite candidate budget, not an assertion that every
// address in a residential provider's unbounded pool has been enumerated.
type Provider struct {
	ID       string
	Attempts int
}
type Request struct {
	Country, Provider string
	Attempt           int
}
type Evaluation struct {
	Health          Health
	Candidate       any
	Reason          string
	ExhaustProvider bool
}
type Probe func(context.Context, Request) Evaluation
type Sweep struct {
	Started        time.Time      `json:"started"`
	Attempts       map[string]int `json:"attempts"`
	ExhaustedUntil time.Time      `json:"exhausted_until"`
}
type Checkpoint struct {
	Scope      string            `json:"scope"`
	RetryAfter time.Time         `json:"retry_after,omitempty"`
	Countries  map[string]*Sweep `json:"countries"`
}
type Decision struct {
	Kind, Country, Provider, Reason string
	Candidate                       any
}
type Selector struct {
	Now                   func() time.Time
	Cooldown, MaxSweepAge time.Duration
}

// Select exhausts providers within a country before considering the next one.
// Interrupted and indeterminate probes never count as evidence of exhaustion.
// Persist the returned checkpoint even on Pending to resume after restart.
// A healthy incumbent is retained by the caller while higher tiers are probed.
func (s Selector) Select(ctx context.Context, scope string, countries []string, providers []Provider, state *Checkpoint, probe Probe) Decision {
	now := s.Now
	if now == nil {
		now = time.Now
	}
	cooldown := s.Cooldown
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	maxAge := s.MaxSweepAge
	if maxAge <= 0 {
		maxAge = 5 * time.Minute
		attempts := 0
		for _, p := range providers {
			n := p.Attempts
			if n < 1 {
				n = 1
			}
			attempts += n
		}
		tiers := 0
		for _, c := range countries {
			if c != Auto {
				tiers++
			}
		}
		// Seven-second probes, incumbent checks and keeper resumption gaps must
		// fit before evidence expires. Unknown results still never exhaust a tier.
		budget := time.Duration(attempts) * time.Duration(tiers) * 30 * time.Second
		if budget > maxAge {
			maxAge = budget
		}
	}
	if state.Scope != scope || state.Countries == nil || (!state.RetryAfter.IsZero() && !state.RetryAfter.After(now())) {
		*state = Checkpoint{Scope: scope, Countries: map[string]*Sweep{}}
	}
	for tier, country := range countries {
		if ctx.Err() != nil {
			return Decision{Kind: "pending", Country: country, Reason: "probe budget interrupted"}
		}
		if country == Auto {
			if !higherExhausted(countries[:tier], state, now()) {
				return Decision{Kind: "pending", Reason: "higher country needs recheck"}
			}
			if state.RetryAfter.IsZero() {
				state.RetryAfter = now().Add(cooldown)
			}
			return Decision{Kind: "legacy", Country: Auto}
		}
		sweep := state.Countries[country]
		at := now()
		if sweep != nil && sweep.ExhaustedUntil.After(at) {
			continue
		}
		if sweep == nil || !sweep.ExhaustedUntil.IsZero() || at.Sub(sweep.Started) > maxAge {
			sweep = &Sweep{Started: at, Attempts: map[string]int{}}
			state.Countries[country] = sweep
		}
		if sweep.Attempts == nil {
			sweep.Attempts = map[string]int{}
		}
		// Round-robin gives every provider a chance before another session on the
		// first provider. Unknown observations do not block healthy peers in this tier.
		uncertain := map[string]bool{}
		for {
			pending := false
			for _, provider := range providers {
				limit := provider.Attempts
				if limit < 1 {
					limit = 1
				}
				attempt := sweep.Attempts[provider.ID]
				if attempt >= limit || uncertain[provider.ID] {
					continue
				}
				pending = true
				if ctx.Err() != nil {
					return Decision{Kind: "pending", Country: country, Reason: "probe budget interrupted"}
				}
				result := probe(ctx, Request{Country: country, Provider: provider.ID, Attempt: attempt})
				if ctx.Err() != nil {
					return Decision{Kind: "pending", Country: country, Reason: "probe budget interrupted"}
				}
				switch result.Health {
				case Usable:
					if !higherExhausted(countries[:tier], state, now()) {
						return Decision{Kind: "pending", Reason: "higher country needs recheck"}
					}
					if state.RetryAfter.IsZero() {
						state.RetryAfter = now().Add(cooldown)
					}
					return Decision{Kind: "selected", Country: country, Provider: provider.ID, Candidate: result.Candidate}
				case Unusable:
					sweep.Attempts[provider.ID] = attempt + 1
					if result.ExhaustProvider {
						sweep.Attempts[provider.ID] = limit
					}
				default:
					uncertain[provider.ID] = true
				}
			}
			if !pending {
				break
			}
		}
		if len(uncertain) > 0 {
			return Decision{Kind: "pending", Country: country, Reason: "country availability unproven"}
		}
		if now().Sub(sweep.Started) > maxAge {
			return Decision{Kind: "pending", Country: country, Reason: "country sweep expired"}
		}
		// Evidence survives a resumable lower-country sweep; retry scheduling is separate.
		sweep.ExhaustedUntil = now().Add(maxAge)
	}
	if state.RetryAfter.IsZero() {
		state.RetryAfter = now().Add(cooldown)
	}
	return Decision{Kind: "exhausted", Reason: "no verified eligible candidate in configured countries"}
}

func higherExhausted(countries []string, state *Checkpoint, now time.Time) bool {
	for _, c := range countries {
		v := state.Countries[c]
		if v == nil || !v.ExhaustedUntil.After(now) {
			return false
		}
	}
	return true
}
