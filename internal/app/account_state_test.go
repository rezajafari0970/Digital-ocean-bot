package app

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
)

func TestClassifyAccountProviderError(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		proxy bool
		want  string
	}{
		{"active", nil, false, ProviderStateActive},
		{"locked", digitalocean.HTTPError{Status: 422, Code: "unprocessable_entity", Message: "There is currently a lock on the account, please log in to the control panel and contact support."}, true, ProviderStateLocked},
		{"token", digitalocean.HTTPError{Status: 401}, false, ProviderStateTokenInvalid},
		{"permission", digitalocean.HTTPError{Status: 403}, false, ProviderStatePermissionDenied},
		{"rate", digitalocean.HTTPError{Status: 429}, false, ProviderStateRateLimited},
		{"proxy", fmt.Errorf("wrapped: %w", network.ErrProxyRequired), true, ProviderStateProxyError},
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
