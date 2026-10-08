package residentialsync

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"testing"
	"time"
)

type delayedRouteCore struct {
	*fakeCore
	delay time.Duration
}

func (f *delayedRouteCore) Do(ctx context.Context, req sanaei.SessionRequest) (sanaei.SessionResponse, error) {
	if req.Path == "panel/api/xray/routeTest" {
		timer := time.NewTimer(f.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return sanaei.SessionResponse{}, ctx.Err()
		case <-timer.C:
		}
	}
	return f.fakeCore.Do(ctx, req)
}

// Reproduces the observed fleet condition: each API request succeeds, but
// aggregate proof latency exceeds the old eight-second startup deadline.
func TestStrictAllowlistSlowNativeProofDoesNotFailOrRepeatMutation(t *testing.T) {
	p := routePolicy{StrictAllowlist: true, Harden: true, AdsOnly: true, Residential: true, Direct: true, Configured: 1, Proxies: []rp{{Type: "socks5", Tag: "residential-ads-test"}}}
	cs, tags := fixtureClients(t, p)
	desired, err := buildSettings(baseSettings(), cs, tags, p)
	if err != nil {
		t.Fatal(err)
	}
	f := &delayedRouteCore{fakeCore: &fakeCore{saved: desired, running: desired}, delay: 300 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = applyAndVerify(ctx, f, desired, desired, "", cs, tags, p); err != nil {
		t.Fatal("healthy slow proof rejected", err)
	}
	if f.saves != 0 || f.restarts != 0 {
		t.Fatal("read-only verification repeated a mutation")
	}
}
