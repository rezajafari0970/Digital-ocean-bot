package app

import "time"

func retryPreviousProxyIP(oldIP string, started time.Time, now time.Time) bool {
	return oldIP != "" && now.Sub(started) < stickySameIPRetryFor
}

func allowCrossCountryFallback(fallback bool, started time.Time, now time.Time) bool {
	return fallback || now.Sub(started) >= stickyFallbackAfter
}
