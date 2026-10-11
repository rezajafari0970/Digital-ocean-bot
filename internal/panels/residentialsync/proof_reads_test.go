package residentialsync

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"sync/atomic"
	"testing"
	"time"
)

type concurrentProofCore struct {
	*delayedRouteCore
	reads, active, peak atomic.Int32
	serial              bool
}

func (f *concurrentProofCore) RouteProofConcurrency() int {
	if f.serial {
		return 1
	}
	return 4
}
func (f *concurrentProofCore) Do(ctx context.Context, r sanaei.SessionRequest) (sanaei.SessionResponse, error) {
	if r.Path == "panel/api/xray/routeTest" {
		f.reads.Add(1)
		n := f.active.Add(1)
		defer f.active.Add(-1)
		for old := f.peak.Load(); n > old; old = f.peak.Load() {
			if f.peak.CompareAndSwap(old, n) {
				break
			}
		}
	}
	return f.delayedRouteCore.Do(ctx, r)
}
func TestNativeProofConcurrencyRetainsCompleteMatrixAndNoMutation(t *testing.T) {
	p := routePolicy{StrictAllowlist: true, Harden: true, AdsOnly: true, Residential: true, Direct: true, Configured: 1, Proxies: []rp{{Type: "socks5", Tag: "residential-ads-test"}}}
	clients, tags := fixtureClients(t, p)
	desired, err := buildSettings(baseSettings(), clients, tags, p)
	if err != nil {
		t.Fatal(err)
	}
	serial := &concurrentProofCore{serial: true, delayedRouteCore: &delayedRouteCore{fakeCore: &fakeCore{saved: desired, running: desired}, delay: 30 * time.Millisecond}}
	before := time.Now()
	if err = applyAndVerify(context.Background(), serial, desired, desired, "", clients, tags, p); err != nil {
		t.Fatal(err)
	}
	serialTime := time.Since(before)
	f := &concurrentProofCore{delayedRouteCore: &delayedRouteCore{fakeCore: &fakeCore{saved: desired, running: desired}, delay: 30 * time.Millisecond}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	if err = applyAndVerify(ctx, f, desired, desired, "", clients, tags, p); err != nil {
		t.Fatal(err)
	}
	expected := int32(len(tags)*3*11 + 1)
	if f.reads.Load() != expected || f.peak.Load() < 2 || f.peak.Load() > 4 || f.active.Load() != 0 || f.saves != 0 || f.restarts != 0 {
		t.Fatal("incomplete or unbounded proof", f.reads.Load(), expected, f.peak.Load())
	}
	if serial.reads.Load() != f.reads.Load() || serial.peak.Load() != 1 {
		t.Fatal("serial and concurrent proof mismatch")
	}
	t.Logf("complete_native_requests=%d serial_duration=%s concurrent_duration=%s peak=%d", expected, serialTime, time.Since(start), f.peak.Load())
}
func TestNativeProofFaultJoinsCancelledReads(t *testing.T) {
	f := &concurrentProofCore{}
	var active atomic.Int32
	failure := errors.New("native proof unavailable")
	checks := []func(context.Context) error{func(context.Context) error { return failure }}
	for i := 0; i < 30; i++ {
		checks = append(checks, func(ctx context.Context) error { active.Add(1); defer active.Add(-1); <-ctx.Done(); return ctx.Err() })
	}
	if err := runRouteProofChecks(context.Background(), f, checks); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if active.Load() != 0 || len(routeProofSlots) != 0 {
		t.Fatal("proof escaped owner cancellation")
	}
}
