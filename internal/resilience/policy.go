package resilience

import "time"

type Policy struct {
	BaseDelay        time.Duration
	MaxDelay         time.Duration
	MaxAttempts      int
	FailureThreshold int
	OpenDuration     time.Duration
}

func DefaultPolicy() Policy {
	return Policy{BaseDelay: time.Second, MaxDelay: 30 * time.Second, MaxAttempts: 5, FailureThreshold: 5, OpenDuration: time.Minute}
}
func (p Policy) Delay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := p.BaseDelay
	if d <= 0 {
		d = time.Second
	}
	max := p.MaxDelay
	if max <= 0 {
		max = 30 * time.Second
	}
	for i := 1; i < attempt && d < max; i++ {
		d *= 2
		if d > max {
			d = max
		}
	}
	return d
}

type ErrorClass string

const (
	Retryable   ErrorClass = "retryable"
	RateLimited ErrorClass = "rate_limited"
	Permanent   ErrorClass = "permanent"
	Unknown     ErrorClass = "unknown"
)
