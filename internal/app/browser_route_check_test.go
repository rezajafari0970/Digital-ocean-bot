package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBrowserRouteConcurrentIdentityChecks(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	entered := make(chan struct{}, 32)
	release := make(chan struct{})
	changed := errors.New("identity changed")
	check := coalesceBrowserRouteChecks(parent, func(ctx context.Context) error {
		calls.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
			return changed
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	var wg sync.WaitGroup
	results := make(chan error, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- check(context.Background()) }()
	}
	<-entered
	// Hold the network observation while parallel browser asset requests arrive.
	time.Sleep(40 * time.Millisecond)
	if n := calls.Load(); n != 1 {
		t.Fatalf("parallel assets opened %d identity probes", n)
	}
	close(release)
	wg.Wait()
	close(results)
	for e := range results {
		if !errors.Is(e, changed) {
			t.Fatal("failed identity observation was not propagated", e)
		}
	}
	before := calls.Load()
	if e := check(context.Background()); !errors.Is(e, changed) || calls.Load() != before+1 {
		t.Fatal("completed observation reused as a cache", e)
	}
	stopped, stop := context.WithCancel(context.Background())
	stop()
	before = calls.Load()
	if e := check(stopped); !errors.Is(e, context.Canceled) || calls.Load() != before {
		t.Fatal("cancelled caller started verification")
	}
}
