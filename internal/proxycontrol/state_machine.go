package proxycontrol

import (
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/resilience"
)

type Policy struct {
	Health                  network.HealthPolicy
	CircuitFailureThreshold int
	OpenDuration            time.Duration
}

func DefaultPolicy() Policy {
	return Policy{
		Health: network.HealthPolicy{
			FailureThreshold:  2,
			RecoveryThreshold: 3,
			MaxHealthyLatency: 5 * time.Second,
		},
		CircuitFailureThreshold: 3,
		OpenDuration:            30 * time.Second,
	}
}

func Allow(x State, now time.Time) (State, error) {
	switch x.CircuitState {
	case "", resilience.Closed:
		x.CircuitState = resilience.Closed
		return x, nil
	case resilience.Open:
		if x.RetryAfter != nil && !now.Before(*x.RetryAfter) {
			x.CircuitState = resilience.HalfOpen
			return x, nil
		}
		return x, resilience.ErrCircuitOpen
	case resilience.HalfOpen:
		return x, nil
	default:
		return x, resilience.ErrCircuitOpen
	}
}

func ApplyHealth(x State, result network.HealthResult, policy Policy) State {
	if policy.CircuitFailureThreshold < 1 {
		policy.CircuitFailureThreshold = 3
	}
	if policy.OpenDuration <= 0 {
		policy.OpenDuration = 30 * time.Second
	}
	hs := network.HealthState{
		Status:               x.HealthState,
		ConsecutiveFailures:  x.ConsecutiveFailures,
		ConsecutiveSuccesses: x.ConsecutiveSuccesses,
		LastCheckedAt:        derefTime(x.LastCheckedAt),
		LastSuccessAt:        derefTime(x.LastSuccessAt),
	}
	hs = hs.Apply(result, policy.Health)
	x.HealthState = hs.Status
	x.ConsecutiveFailures = hs.ConsecutiveFailures
	x.ConsecutiveSuccesses = hs.ConsecutiveSuccesses
	x.LastCheckedAt = timePtr(hs.LastCheckedAt)
	if !hs.LastSuccessAt.IsZero() {
		x.LastSuccessAt = timePtr(hs.LastSuccessAt)
	}

	if hs.Status == network.StatusHealthy {
		x.CircuitState = resilience.Closed
		x.RetryAfter = nil
		x.LastErrorClass = ""
		x.LastErrorDetail = ""
		return x
	}

	if result.Error != "" {
		x.LastErrorClass = "PROXY_HEALTH_FAILED"
		x.LastErrorDetail = result.Error
	}

	if x.CircuitState == resilience.HalfOpen || x.ConsecutiveFailures >= policy.CircuitFailureThreshold {
		x.CircuitState = resilience.Open
		retry := result.CheckedAt.Add(policy.OpenDuration)
		x.RetryAfter = &retry
	}
	return x
}

func BumpGeneration(x State) State {
	if x.Generation < 1 {
		x.Generation = 1
	}
	x.Generation++
	return x
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	v := t
	return &v
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
