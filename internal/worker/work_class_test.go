package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPollClassCannotOccupyRoutingHeadroom(t *testing.T) {
	shared := NewWorkBudget(10)
	polls := NewWorkClass(shared, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pollCtx, stopPolls := context.WithCancel(ctx)
	defer stopPolls()
	pollStarted := make(chan struct{}, 20)
	routesStarted := make(chan struct{}, 8)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	results := make(chan error, 28)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- polls.Do(pollCtx, func(context.Context) error { pollStarted <- struct{}{}; <-release; return nil })
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-pollStarted:
		case <-ctx.Done():
			t.Fatal("poll class failed to start")
		}
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- shared.Do(ctx, func(context.Context) error { routesStarted <- struct{}{}; <-release; return nil })
		}()
	}
	for i := 0; i < 8; i++ {
		select {
		case <-routesStarted:
		case <-ctx.Done():
			t.Fatal("background queue occupied proof headroom")
		}
	}
	if shared.Snapshot()["active"] != 10 || polls.Snapshot()["admitted"] != 2 {
		t.Fatal("shared/class limits changed")
	}
	stopPolls()
	// Cancelling queued polls must not abandon either active poll's permit.
	select {
	case <-pollStarted:
		t.Fatal("a third poll overlapped")
	default:
	}
	if polls.Snapshot()["admitted"] != 2 {
		t.Fatal("cancellation released active handler")
	}
	close(release)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	if shared.Snapshot()["active"] != 0 || polls.Snapshot()["admitted"] != 0 || polls.Snapshot()["waiting"] != 0 {
		t.Fatal("admission slot leaked")
	}
}
