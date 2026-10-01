package proxycontrol

import (
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/resilience"
)

func TestDefaultStateIsProviderProxyScopedAndFailNeutral(t *testing.T) {
	x := DefaultState("account-a", "proxy-a", "vultr")
	if x.AccountID != "account-a" || x.ProxyID != "proxy-a" || x.Provider != "vultr" {
		t.Fatalf("scope lost: %+v", x)
	}
	if x.HealthState != network.StatusUnknown || x.CircuitState != resilience.Closed {
		t.Fatalf("unexpected initial state: %+v", x)
	}
	if x.Generation != 1 || x.ConsecutiveFailures != 0 || x.ConsecutiveSuccesses != 0 {
		t.Fatalf("unexpected counters: %+v", x)
	}
}
