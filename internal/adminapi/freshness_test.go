package adminapi

import (
	"testing"
	"time"
)

func TestObservationFreshness(t *testing.T) {
	now := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"never", time.Time{}, "never"},
		{"fresh", now.Add(-90 * time.Second), "fresh"},
		{"fresh boundary", now.Add(-2 * time.Minute), "fresh"},
		{"stale", now.Add(-3 * time.Minute), "stale"},
		{"stale boundary", now.Add(-5 * time.Minute), "stale"},
		{"expired", now.Add(-6 * time.Minute), "expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := observationFreshness(tt.at, now); got != tt.want {
				t.Fatalf("got=%s want=%s", got, tt.want)
			}
		})
	}
}
