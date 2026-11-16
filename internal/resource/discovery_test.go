package resource

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiscoveryCoalescesIdleAndReopensAtCadence(t *testing.T) {
	var gate Discovery
	now := time.Now()
	var calls atomic.Int64
	run := func(context.Context, func()) error { calls.Add(1); return nil }
	for round := 0; round < 10; round++ {
		var wg sync.WaitGroup
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := gate.Do(context.Background(), now.Add(time.Duration(round)*time.Second), time.Second, run); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
	}
	if calls.Load() != 10 {
		t.Fatalf("idle scans=%d want 10 instead of 640", calls.Load())
	}
}
func TestDiscoveryClaimOpensCapacityBeforeCompletion(t *testing.T) {
	var gate Discovery
	now := time.Now()
	foundWork := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- gate.Do(context.Background(), now, time.Second, func(_ context.Context, found func()) error { found(); close(foundWork); <-release; return nil })
	}()
	<-foundWork
	var wg sync.WaitGroup
	var calls atomic.Int64
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gate.Do(context.Background(), now, time.Second, func(_ context.Context, found func()) error { calls.Add(1); found(); return nil })
		}()
	}
	wg.Wait()
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 8 {
		t.Fatalf("work serialized behind slow execution: %d", calls.Load())
	}
}
func TestDiscoveryStaleEmptyCannotOverrideNewClaim(t *testing.T) {
	var gate Discovery
	now := time.Now()
	gate.Do(context.Background(), now, time.Second, func(_ context.Context, f func()) error { f(); return nil })
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- gate.Do(context.Background(), now, time.Second, func(context.Context, func()) error { close(started); <-release; return nil })
	}()
	<-started
	gate.Do(context.Background(), now, time.Second, func(_ context.Context, f func()) error { f(); return nil })
	close(release)
	<-done
	called := false
	gate.Do(context.Background(), now, time.Second, func(_ context.Context, f func()) error { called = true; f(); return nil })
	if !called {
		t.Fatal("stale empty discovery suppressed new work")
	}
}
func TestDiscoveryCancellationFailurePanicAndClassIsolation(t *testing.T) {
	var gate, other Discovery
	now := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := gate.Do(ctx, now, time.Second, func(context.Context, func()) error { t.Fatal("cancelled dispatch"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	failure := errors.New("database unavailable")
	if err := gate.Do(context.Background(), now, time.Second, func(context.Context, func()) error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	called := false
	other.Do(context.Background(), now, time.Second, func(context.Context, func()) error { called = true; return nil })
	if !called {
		t.Fatal("unrelated class blocked")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic was hidden")
			}
		}()
		gate.Do(context.Background(), now.Add(time.Second), time.Second, func(context.Context, func()) error { panic("fixture") })
	}()
	gate.Do(context.Background(), now.Add(2*time.Second), time.Second, func(_ context.Context, f func()) error { f(); return nil })
}
func TestDiscoveryLateClaimDoesNotReopenFinishedProbe(t *testing.T) {
	var gate Discovery
	now := time.Now()
	var late func()
	gate.Do(context.Background(), now, time.Second, func(_ context.Context, f func()) error { late = f; return nil })
	late()
	gate.Do(context.Background(), now, time.Second, func(context.Context, func()) error { t.Fatal("late callback reopened gate"); return nil })
}
