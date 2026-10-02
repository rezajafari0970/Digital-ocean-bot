package app

import (
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestProviderStateFaultMatrix(t *testing.T) {
	tests := []struct {
		class providers.ErrorClass
		msg   string
		proxy bool
		want  string
	}{
		{providers.ErrorAccountLocked, "account locked", false, ProviderStateLocked},
		{providers.ErrorAuthentication, "bad token", false, ProviderStateTokenInvalid},
		{providers.ErrorPermissionDenied, "forbidden", false, ProviderStatePermissionDenied},
		{providers.ErrorPermissionDenied, "outstanding balance", false, ProviderStateBillingBlocked},
		{providers.ErrorRateLimited, "rate limit", false, ProviderStateRateLimited},
		{providers.ErrorTransport, "dial timeout", false, ProviderStateTransportError},
		{providers.ErrorUnavailable, "upstream unavailable", false, ProviderStateTransportError},
		{providers.ErrorAmbiguousOutcome, "unknown create outcome", false, ProviderStateTransportError},
	}
	for _, tc := range tests {
		err := &providers.Error{Class: tc.class, Message: tc.msg}
		if got := ClassifyAccountProviderError(err, tc.proxy); got != tc.want {
			t.Fatalf("class=%s msg=%q got=%s want=%s", tc.class, tc.msg, got, tc.want)
		}
	}
}

func TestProviderProbeIntervals(t *testing.T) {
	if got := ProviderProbeInterval(ProviderStateLocked); got <= 0 || got > time.Minute {
		t.Fatalf("LOCKED must recover quickly, got=%s", got)
	}
	tests := map[string]time.Duration{
		ProviderStateTokenInvalid:     5 * time.Minute,
		ProviderStatePermissionDenied: 5 * time.Minute,
		ProviderStateBillingBlocked:   5 * time.Minute,
		ProviderStateRateLimited:      2 * time.Minute,
		ProviderStateTransportError:   15 * time.Second,
		ProviderStateProxyError:       15 * time.Second,
		ProviderStateActive:           0,
	}
	for state, want := range tests {
		if got := ProviderProbeInterval(state); got != want {
			t.Fatalf("state=%s got=%s want=%s", state, got, want)
		}
	}
}
