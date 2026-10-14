package app

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestClassifyAccountProviderError(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		proxy bool
		want  string
	}{
		{"active", nil, false, ProviderStateActive},
		{"locked", &providers.Error{Class: providers.ErrorAccountLocked}, true, ProviderStateLocked},
		{"token", &providers.Error{Class: providers.ErrorAuthentication}, false, ProviderStateTokenInvalid},
		{"permission", &providers.Error{Class: providers.ErrorPermissionDenied}, false, ProviderStatePermissionDenied},
		{"billing", &providers.Error{Class: providers.ErrorPermissionDenied, Message: "There is currently an outstanding balance on your account, please visit the billing page to update your billing profile."}, false, ProviderStateBillingBlocked},
		{"rate", &providers.Error{Class: providers.ErrorRateLimited}, false, ProviderStateRateLimited},
		{"proxy", fmt.Errorf("wrapped: %w", network.ErrProxyRequired), true, ProviderStateProxyError},
		{"proxy-circuit", network.ErrProxyCircuitOpen, true, ProviderStateProxyError},
		{"local-network", ErrNetworkNotReady, true, ProviderStateProxyError},
		{"wrapped-local-network", fmt.Errorf("runtime: %w", ErrNetworkNotReady), true, ProviderStateProxyError},
		{"direct-not-proxy", ErrNetworkNotReady, false, ProviderStateTransportError},
		{"transport", errors.New("connection reset"), false, ProviderStateTransportError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyAccountProviderError(tc.err, tc.proxy); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
func TestProviderStateRecoveryPolicy(t *testing.T) {
	if !errors.Is(ErrNetworkNotReady, network.ErrAccountNetworkNotReady) {
		t.Fatal("network sentinel split")
	}
	if providerObservationProbeInterval(ProviderStateLocked, ProviderStateProxyError) != 45*time.Second {
		t.Fatal("proxy error accelerated lock probes")
	}
	if providerObservationProbeInterval(ProviderStateTokenInvalid, ProviderStateTransportError) != 5*time.Minute {
		t.Fatal("transport error accelerated credential probes")
	}
	if ProviderProbeInterval(ProviderStateLocked) > time.Minute {
		t.Fatal("locked accounts must be reprobed quickly")
	}
	if IsProviderObservationError(ProviderStateLocked) {
		t.Fatal("lock is semantic provider state")
	}
	if !IsProviderObservationError(ProviderStateTransportError) || !IsProviderObservationError(ProviderStateProxyError) {
		t.Fatal("transport/proxy must stay separate")
	}
}
