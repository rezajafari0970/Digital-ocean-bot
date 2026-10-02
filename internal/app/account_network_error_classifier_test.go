package app

import (
	"fmt"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

func TestProxyRequiredNetworkFailuresClassifyAsProxyError(t *testing.T) {
	errs := []error{
		network.ErrProxyRequired,
		network.ErrProxyUnavailable,
		network.ErrProxyConfigInvalid,
		network.ErrAccountNetworkNotReady,
		network.ErrProxyCircuitOpen,
		network.ErrProxyAuth,
		network.ErrProxyDNS,
		network.ErrProxyConnect,
		network.ErrIPv6Prohibited,
		network.ErrIPv4Required,
		network.ErrUnexpectedExitIP,
		network.ErrServerIPLeak,
		network.ErrEgressChanged,
		network.ErrAccountContextMismatch,
	}
	for _, base := range errs {
		err := fmt.Errorf("wrapped: %w", base)
		if got := ClassifyAccountProviderError(err, true); got != ProviderStateProxyError {
			t.Fatalf("err=%v got=%s", base, got)
		}
	}
}

func TestNetworkFailureWithoutProxyRequirementIsTransport(t *testing.T) {
	if got := ClassifyAccountProviderError(fmt.Errorf("wrapped: %w", network.ErrProxyConnect), false); got != ProviderStateTransportError {
		t.Fatalf("got=%s", got)
	}
}
