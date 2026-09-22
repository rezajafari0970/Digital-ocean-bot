package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryEventuallySucceeds(t *testing.T) {
	calls := 0
	r := Retrier{Policy: Policy{BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond, MaxAttempts: 4, FailureThreshold: 10}, Classify: func(error) ErrorClass { return Retryable }}
	err := r.Do(context.Background(), func() error {
		calls++
		if calls < 3 {
			return errors.New("temporary")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
func TestPermanentDoesNotRetry(t *testing.T) {
	calls := 0
	r := Retrier{Policy: DefaultPolicy(), Classify: func(error) ErrorClass { return Permanent }}
	err := r.Do(context.Background(), func() error { calls++; return errors.New("bad") })
	if err == nil || calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}
func TestCircuitOpens(t *testing.T) {
	c := &Circuit{Policy: Policy{FailureThreshold: 2, OpenDuration: time.Minute}}
	now := time.Now()
	c.Failure(now)
	c.Failure(now)
	if err := c.Allow(now.Add(time.Second)); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected open got %v", err)
	}
}
