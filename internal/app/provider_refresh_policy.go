package app

import "time"

func providerRefreshMaySkip(fresh bool, providerState, providerError string, providerCheckedAt time.Time, maxAge time.Duration, now time.Time) bool {
	if !fresh || providerState != ProviderStateActive || providerError != "" || providerCheckedAt.IsZero() {
		return false
	}
	if maxAge <= 0 {
		maxAge = 90 * time.Second
	}
	age := now.Sub(providerCheckedAt)
	return age >= 0 && age <= maxAge
}
