package network

import "time"

type HealthPolicy struct {
	FailureThreshold  int
	RecoveryThreshold int
	MaxHealthyLatency time.Duration
}

type HealthState struct {
	Status               ProxyStatus
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	LastLatency          time.Duration
	LastExitIP           string
	LastCheckedAt        time.Time
	LastSuccessAt        time.Time
}

func (s HealthState) Apply(result HealthResult, policy HealthPolicy) HealthState {
	s.LastCheckedAt = result.CheckedAt
	s.LastLatency = result.Latency
	s.LastExitIP = result.ExitIP
	if policy.FailureThreshold < 1 {
		policy.FailureThreshold = 2
	}
	if policy.RecoveryThreshold < 1 {
		policy.RecoveryThreshold = 2
	}
	if result.Status == StatusHealthy && (policy.MaxHealthyLatency <= 0 || result.Latency <= policy.MaxHealthyLatency) {
		s.ConsecutiveFailures = 0
		s.ConsecutiveSuccesses++
		s.LastSuccessAt = result.CheckedAt
		if s.ConsecutiveSuccesses >= policy.RecoveryThreshold {
			s.Status = StatusHealthy
		} else {
			s.Status = StatusDegraded
		}
		return s
	}
	s.ConsecutiveSuccesses = 0
	s.ConsecutiveFailures++
	if s.ConsecutiveFailures >= policy.FailureThreshold {
		s.Status = StatusDown
	} else {
		s.Status = StatusDegraded
	}
	return s
}
