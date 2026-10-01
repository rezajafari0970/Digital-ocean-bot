package adminapi

import "time"

func observationFreshness(at time.Time, now time.Time) string {
	if at.IsZero() {
		return "never"
	}
	age := now.Sub(at)
	if age <= 2*time.Minute {
		return "fresh"
	}
	if age <= 5*time.Minute {
		return "stale"
	}
	return "expired"
}
