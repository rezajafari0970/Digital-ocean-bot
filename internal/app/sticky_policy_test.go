package app

import (
	"testing"
	"time"
)

func TestStickyRecoveryWindows(t *testing.T) {
	start := time.Unix(1000, 0)
	if !retryPreviousProxyIP("203.0.113.1", start, start.Add(119*time.Second)) {
		t.Fatal("previous IP must be retried during first two minutes")
	}
	if retryPreviousProxyIP("203.0.113.1", start, start.Add(2*time.Minute)) {
		t.Fatal("same-IP-only window must end at two minutes")
	}
	if allowCrossCountryFallback(false, start, start.Add(299*time.Second)) {
		t.Fatal("cross-country fallback before five minutes")
	}
	if !allowCrossCountryFallback(false, start, start.Add(5*time.Minute)) {
		t.Fatal("cross-country fallback must open at five minutes")
	}
}
