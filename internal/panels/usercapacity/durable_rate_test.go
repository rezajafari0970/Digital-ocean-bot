package usercapacity

import (
	"testing"
	"time"
)

func TestDurableRateStartsWithoutRestartBurst(t *testing.T) {
	now := time.Unix(1000, 0)
	tokens, n := durableRateStep(0, now, 100, 10000, now)
	if n != 0 || tokens != 0 {
		t.Fatalf("n=%d tokens=%v", n, tokens)
	}
	_, n = durableRateStep(tokens, now, 100, 10000, now.Add(250*time.Millisecond))
	if n != 25 {
		t.Fatalf("n=%d want 25", n)
	}
}

func TestDurableRateCapsCatchupAtOneSecond(t *testing.T) {
	now := time.Unix(1000, 0)
	tokens, n := durableRateStep(0, now, 100, 10000, now.Add(10*time.Second))
	if n != 100 || tokens != 0 {
		t.Fatalf("n=%d tokens=%v", n, tokens)
	}
}

func TestDurableRateHonorsPersistedFractionAcrossRestart(t *testing.T) {
	last := time.Unix(1000, 0)
	tokens, n := durableRateStep(0.5, last, 10, 100, last.Add(50*time.Millisecond))
	if n != 1 {
		t.Fatalf("n=%d want 1", n)
	}
	if tokens < 0 || tokens >= 1 {
		t.Fatalf("remaining=%v", tokens)
	}
}

func TestDurableRateClockRollbackDoesNotMintTokens(t *testing.T) {
	now := time.Unix(1000, 0)
	tokens, n := durableRateStep(0, now, 10, 100, now.Add(-time.Second))
	if n != 0 || tokens != 0 {
		t.Fatalf("n=%d tokens=%v", n, tokens)
	}
}
