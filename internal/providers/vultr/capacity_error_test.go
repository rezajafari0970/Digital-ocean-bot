package vultr

import (
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestCreateAccountInstanceLimitIsCapacity(t *testing.T) {
	err := normalizeError("create_server", HTTPError{
		Status:  400,
		Message: "Unable to place order: You have reached the maximum number of active instances for this account",
	})
	if !providers.IsClass(err, providers.ErrorCapacity) {
		t.Fatalf("class=%s err=%v", providers.Class(err), err)
	}
}

func TestCreateRegionShortageIsRegionCapacity(t *testing.T) {
	err := normalizeError("create_server", HTTPError{
		Status:  409,
		Message: "Selected plan is not available due to capacity in this region",
	})
	if !providers.IsClass(err, providers.ErrorRegionCapacity) {
		t.Fatalf("class=%s err=%v", providers.Class(err), err)
	}
}

func TestGenericRateLimitNeverBecomesComputeCapacity(t *testing.T) {
	err := normalizeError("create_server", HTTPError{
		Status:  429,
		Message: "rate limit exceeded",
	})
	if !providers.IsClass(err, providers.ErrorRateLimited) {
		t.Fatalf("class=%s err=%v", providers.Class(err), err)
	}
}

func TestAccountLimitPhraseOutsideCreateDoesNotBecomeCapacity(t *testing.T) {
	err := normalizeError("health", HTTPError{
		Status:  400,
		Message: "You have reached the maximum number of active instances for this account",
	})
	if providers.IsClass(err, providers.ErrorCapacity) {
		t.Fatalf("health error incorrectly classified as capacity: %v", err)
	}
}
