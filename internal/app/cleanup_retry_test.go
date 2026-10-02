package app

import (
	"errors"
	"testing"
	"time"
)

func TestSlowCleanupProviderErrorOnlyForExternalBlocks(t *testing.T) {
	base := errors.New("provider blocked")
	for _, state := range []string{ProviderStateLocked, ProviderStateBillingBlocked} {
		err := slowCleanupProviderError(state, base)
		h, ok := err.(interface{ RetryDelay() time.Duration })
		if !ok || h.RetryDelay() != 10*time.Minute {
			t.Fatalf("state=%s missing 10m retry hint", state)
		}
	}
	if got := slowCleanupProviderError(ProviderStateTransportError, base); got != base {
		t.Fatal("transport cleanup errors must keep normal fast backoff")
	}
}
