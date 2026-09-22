package network

import (
	"testing"
	"time"
)

func TestHealthStateUsesHysteresis(t *testing.T) {
	p := HealthPolicy{FailureThreshold: 2, RecoveryThreshold: 2, MaxHealthyLatency: time.Second}
	now := time.Now()
	s := HealthState{Status: StatusHealthy}
	s = s.Apply(HealthResult{Status: StatusDown, CheckedAt: now}, p)
	if s.Status != StatusDegraded {
		t.Fatal("first failure should degrade")
	}
	s = s.Apply(HealthResult{Status: StatusDown, CheckedAt: now.Add(time.Second)}, p)
	if s.Status != StatusDown {
		t.Fatal("second failure should mark down")
	}
	s = s.Apply(HealthResult{Status: StatusHealthy, CheckedAt: now.Add(2 * time.Second), Latency: time.Millisecond}, p)
	if s.Status != StatusDegraded {
		t.Fatal("first recovery should degrade")
	}
	s = s.Apply(HealthResult{Status: StatusHealthy, CheckedAt: now.Add(3 * time.Second), Latency: time.Millisecond}, p)
	if s.Status != StatusHealthy {
		t.Fatal("second recovery should mark healthy")
	}
}
