package app

import (
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestProviderMessageMentioningProxyIsNotAutomaticallyProxyError(t *testing.T) {
	err := &providers.Error{
		Class:     providers.ErrorPermissionDenied,
		Operation: "observe",
		Message:   "proxy access is not permitted for this account",
	}
	if got := ClassifyAccountProviderError(err, true); got != ProviderStatePermissionDenied {
		t.Fatalf("got %q want %q", got, ProviderStatePermissionDenied)
	}
}
