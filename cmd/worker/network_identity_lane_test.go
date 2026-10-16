package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIdentityReservedCapacityWithBulkAndBothProviders(t *testing.T) {
	parent, bulk, provider, identity := newControlWorkBudgets()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{}, 5)
	release := make(chan struct{})
	done := make(chan struct{}, 6)
	job := func(context.Context) error { entered <- struct{}{}; <-release; return nil }
	launch := func(admit func(context.Context, func(context.Context) error) error) {
		go func() { _ = admit(ctx, job); done <- struct{}{} }()
	}
	launch(bulk.Do)
	launch(provider.Do)
	launch(provider.Do)
	launch(identity.Do)
	launch(identity.Do)
	for range 5 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("an observation class starved")
		}
	}
	launch(bulk.Do)
	if parent.Snapshot()["active"] != 5 || identity.Snapshot()["admitted"] != 2 {
		t.Error("aggregate/class bounds", parent.Snapshot(), identity.Snapshot())
	}
	cancel()
	// Cancellation cannot release executing owners before their cleanup returns.
	if parent.Snapshot()["active"] != 5 {
		t.Error("active ownership released before join")
	}
	close(release)
	for range 6 {
		<-done
	}
	if parent.Snapshot()["active"] != 0 {
		t.Fatal("permit leaked")
	}
}

func TestIdentityDeadlineStartsAfterAdmission(t *testing.T) {
	_, _, _, identity := newControlWorkBudgets()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	holds := make(chan struct{}, 2)
	release := make(chan struct{})
	var holders sync.WaitGroup
	for range 2 {
		holders.Add(1)
		go func() {
			defer holders.Done()
			_ = identity.Do(ctx, func(context.Context) error { holds <- struct{}{}; <-release; return nil })
		}()
	}
	<-holds
	<-holds
	called := make(chan time.Duration, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runNetworkIdentities(ctx, identity.Do, func(context.Context) ([]string, error) { return []string{"a"}, nil }, func(ctx context.Context, _ string) error {
			deadline, _ := ctx.Deadline()
			called <- time.Until(deadline)
			return nil
		}, identityScheduleOptions{100 * time.Millisecond, time.Millisecond, time.Hour})
	}()
	time.Sleep(150 * time.Millisecond)
	close(release)
	holders.Wait()
	select {
	case remaining := <-called:
		if remaining < 80*time.Millisecond {
			t.Errorf("queue consumed operation deadline: %s", remaining)
		}
	case <-time.After(time.Second):
		t.Error("identity expired while waiting")
	}
	cancel()
	<-done
}

func TestIdentityRollingSlowPeerDoesNotBlockHealthyTail(t *testing.T) {
	_, _, _, identity := newControlWorkBudgets()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ids := []string{"slow"}
	for i := 0; i < 9; i++ {
		ids = append(ids, fmt.Sprint(i))
	}
	slow := make(chan struct{})
	cleanup := make(chan struct{})
	seen := make(chan string, 20)
	done := make(chan struct{})
	var active, maxActive atomic.Int32
	go func() {
		defer close(done)
		runNetworkIdentities(ctx, identity.Do, func(context.Context) ([]string, error) { return ids, nil }, func(ctx context.Context, id string) error {
			n := active.Add(1)
			defer active.Add(-1)
			for old := maxActive.Load(); n > old; old = maxActive.Load() {
				if maxActive.CompareAndSwap(old, n) {
					break
				}
			}
			if id == "slow" {
				close(slow)
				<-ctx.Done()
				<-cleanup
				return ctx.Err()
			}
			seen <- id
			return nil
		}, identityScheduleOptions{time.Minute, time.Millisecond, time.Hour})
	}()
	<-slow
	found := map[string]bool{}
	for len(found) < 9 {
		select {
		case id := <-seen:
			if found[id] {
				t.Error("duplicate scheduling", id)
			}
			found[id] = true
		case <-time.After(2 * time.Second):
			cancel()
			close(cleanup)
			<-done
			t.Fatal("healthy tail blocked behind slow account")
		}
	}
	cancel()
	select {
	case <-done:
		t.Error("coordinator returned before handler cleanup")
	case <-time.After(20 * time.Millisecond):
	}
	close(cleanup)
	<-done
	if maxActive.Load() > 2 || active.Load() != 0 {
		t.Fatal("unbounded or unjoined handlers")
	}
}

func TestIdentityDiscoveryFailureBackoffAndCancellation(t *testing.T) {
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runNetworkIdentities(ctx, func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }, func(context.Context) ([]string, error) {
			calls.Add(1)
			return nil, fmt.Errorf("temporary database failure")
		}, func(context.Context, string) error { t.Error("handler after failed discovery"); return nil }, identityScheduleOptions{time.Second, 10 * time.Millisecond, time.Second})
	}()
	time.Sleep(120 * time.Millisecond)
	cancel()
	<-done
	if n := calls.Load(); n < 1 || n > 3 {
		t.Fatal("discovery retry storm", n)
	}
}
