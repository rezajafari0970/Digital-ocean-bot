package proxycontrol

import (
	"errors"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/resilience"
)

func TestProxyControlCircuitOpensAndRecoversWithHysteresis(t *testing.T) {
	p := DefaultPolicy()
	p.CircuitFailureThreshold = 2
	p.OpenDuration = time.Minute
	p.Health.FailureThreshold = 2
	p.Health.RecoveryThreshold = 2
	now := time.Now().UTC()

	x := DefaultState("a", "p", "vultr")
	fail := network.HealthResult{Status: network.StatusDown, CheckedAt: now, Error: "timeout"}
	x = ApplyHealth(x, fail, p)
	if x.CircuitState != resilience.Closed || x.HealthState != network.StatusDegraded {
		t.Fatalf("first failure: %+v", x)
	}
	fail.CheckedAt = now.Add(time.Second)
	x = ApplyHealth(x, fail, p)
	if x.CircuitState != resilience.Open || x.HealthState != network.StatusDown || x.RetryAfter == nil {
		t.Fatalf("second failure must open: %+v", x)
	}
	if _, err := Allow(x, now.Add(30*time.Second)); !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("open circuit allowed early: %v", err)
	}
	x, err := Allow(x, now.Add(2*time.Minute))
	if err != nil || x.CircuitState != resilience.HalfOpen {
		t.Fatalf("half-open transition: state=%s err=%v", x.CircuitState, err)
	}
	ok := network.HealthResult{Status: network.StatusHealthy, Latency: time.Millisecond, CheckedAt: now.Add(2*time.Minute + time.Second)}
	x = ApplyHealth(x, ok, p)
	if x.CircuitState != resilience.Open && x.CircuitState != resilience.HalfOpen {
		t.Fatalf("single recovery probe must not fully recover health: %+v", x)
	}
	ok.CheckedAt = now.Add(2*time.Minute + 2*time.Second)
	x = ApplyHealth(x, ok, p)
	if x.HealthState != network.StatusHealthy || x.CircuitState != resilience.Closed || x.RetryAfter != nil {
		t.Fatalf("recovery failed: %+v", x)
	}
}

func TestProxyGenerationIsMonotonic(t *testing.T) {
	x := DefaultState("a", "p", "do")
	if x.Generation != 1 {
		t.Fatal(x.Generation)
	}
	x = BumpGeneration(x)
	x = BumpGeneration(x)
	if x.Generation != 3 {
		t.Fatalf("generation=%d", x.Generation)
	}
}
