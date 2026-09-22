package resilience

import (
	"errors"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker open")

type CircuitState string

const (
	Closed   CircuitState = "closed"
	Open     CircuitState = "open"
	HalfOpen CircuitState = "half_open"
)

type Circuit struct {
	mu       sync.Mutex
	State    CircuitState
	Failures int
	OpenedAt time.Time
	Policy   Policy
}

func (c *Circuit) Allow(now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.State == "" {
		c.State = Closed
	}
	if c.State == Open {
		if now.Sub(c.OpenedAt) >= c.Policy.OpenDuration {
			c.State = HalfOpen
			return nil
		}
		return ErrCircuitOpen
	}
	return nil
}
func (c *Circuit) Success() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.State = Closed
	c.Failures = 0
	c.OpenedAt = time.Time{}
}
func (c *Circuit) Failure(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Failures++
	threshold := c.Policy.FailureThreshold
	if threshold < 1 {
		threshold = 5
	}
	if c.Failures >= threshold || c.State == HalfOpen {
		c.State = Open
		c.OpenedAt = now
	}
}
