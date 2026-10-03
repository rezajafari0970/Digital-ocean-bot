package usercapacity

import (
	"testing"
	"time"
)

func TestBulkRateLimiterUsesWallClockBudget(t *testing.T) {
	var l bulkRateLimiter
	now := time.Unix(1000, 0)

	if got := l.allowance("p/1", 100, 10000, now); got != 100 {
		t.Fatalf("initial one-second burst=%d want 100", got)
	}
	if got := l.allowance("p/1", 100, 10000, now); got != 0 {
		t.Fatalf("same-instant allowance=%d want 0", got)
	}
	if got := l.allowance("p/1", 100, 10000, now.Add(250*time.Millisecond)); got != 25 {
		t.Fatalf("250ms allowance=%d want 25", got)
	}
	if got := l.allowance("p/1", 100, 10000, now.Add(time.Second)); got != 75 {
		t.Fatalf("remaining one-second allowance=%d want 75", got)
	}
}

func TestBulkRateLimiterCapsDelayedBurst(t *testing.T) {
	var l bulkRateLimiter
	now := time.Unix(2000, 0)
	_ = l.allowance("p/1", 100, 10000, now)
	if got := l.allowance("p/1", 100, 10000, now.Add(10*time.Second)); got != 100 {
		t.Fatalf("delayed cycle burst=%d want cap 100", got)
	}
}

func TestBulkRateLimiterIsolatesInboundsAndDeficit(t *testing.T) {
	var l bulkRateLimiter
	now := time.Unix(3000, 0)
	if got := l.allowance("p/1", 100, 7, now); got != 7 {
		t.Fatalf("deficit cap=%d want 7", got)
	}
	if got := l.allowance("p/2", 100, 10000, now); got != 100 {
		t.Fatalf("second inbound must have independent budget, got %d", got)
	}
}

func TestBulkRateLimiterClockRollbackDoesNotMintTokens(t *testing.T) {
	var l bulkRateLimiter
	now := time.Unix(4000, 0)
	_ = l.allowance("p/1", 100, 10000, now)
	if got := l.allowance("p/1", 100, 10000, now.Add(-time.Second)); got != 0 {
		t.Fatalf("clock rollback minted %d tokens", got)
	}
}
