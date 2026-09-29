package adminapi

import (
	"testing"
	"time"
)

func TestOutputVisibleUntilTenSecondsBeforeExpiry(t *testing.T) {
	expiry := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	got := outputVisibleUntil(float64(expiry.UnixMilli()))
	want := expiry.Add(-10 * time.Second)
	if got == nil || !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestOutputVisibleUntilNoExpiry(t *testing.T) {
	if got := outputVisibleUntil(float64(0)); got != nil {
		t.Fatalf("got %v want nil", got)
	}
	if got := outputVisibleUntil(nil); got != nil {
		t.Fatalf("got %v want nil", got)
	}
}
