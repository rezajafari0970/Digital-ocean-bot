package adminapi

import "time"

func observationFreshness(at time.Time, now time.Time) string {
	if at.IsZero() || at.After(now) {
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

// Display-only grace is bounded to the real scheduler cadence plus one tick.
func withinObservationSchedule(at, now time.Time, seconds int) bool {
	return !at.IsZero() && !at.After(now) && seconds > 90 && seconds <= 300 && now.Sub(at) <= time.Duration(seconds+30)*time.Second
}

// The display retains scheduled evidence without changing the strict freshness
// window used for mutation admission.
func observationDisplaySeconds(seconds int) int {
	if seconds > 90 && seconds <= 300 {
		return max(120, seconds+30)
	}
	return 120
}
