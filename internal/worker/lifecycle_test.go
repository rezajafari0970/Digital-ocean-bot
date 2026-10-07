package worker

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"sync/atomic"
	"testing"
	"time"
)

type lifecycleSourceStub struct{ items []droplets.LifecycleItem }

func (s lifecycleSourceStub) Due(context.Context, time.Time, int) ([]droplets.LifecycleItem, error) {
	return s.items, nil
}

type lifecycleHandlerFunc func(context.Context, droplets.LifecycleItem) error

func (f lifecycleHandlerFunc) ProcessLifecycle(ctx context.Context, i droplets.LifecycleItem) error {
	return f(ctx, i)
}
func TestLifecycleIndependentAccountAndNoOverlapAfterDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	canceled := make(chan struct{})
	release := make(chan struct{})
	healthy := make(chan struct{}, 10)
	var aCalls atomic.Int32
	d := NewDispatcher(2, 1)
	w := LifecycleWorker{
		Dispatcher: d,
		Store:      lifecycleSourceStub{[]droplets.LifecycleItem{{ID: "a1", AccountID: "a"}, {ID: "a2", AccountID: "a"}, {ID: "b1", AccountID: "b"}}},
		Interval:   5 * time.Millisecond, ItemTimeout: 30 * time.Millisecond, Concurrency: 2,
		Handler: lifecycleHandlerFunc(func(ctx context.Context, i droplets.LifecycleItem) error {
			if i.AccountID == "a" {
				if aCalls.Add(1) == 1 {
					<-ctx.Done()
					close(canceled)
					<-release
				}
				return ctx.Err()
			}
			select {
			case healthy <- struct{}{}:
			default:
			}
			return nil
		}),
	}
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	select {
	case <-healthy:
	case <-time.After(time.Second):
		t.Fatal("healthy account blocked")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("item has no deadline")
	}
	// The timed-out handler deliberately has not returned. Its key/account must
	// remain occupied across later polls; another account can still advance.
	select {
	case <-healthy:
	case <-time.After(time.Second):
		t.Fatal("healthy stopped after sibling timeout")
	}
	if s := d.Snapshot(20 * time.Millisecond); s.Stalled != 1 {
		t.Fatal("stalled handler hidden", s)
	}
	if aCalls.Load() != 1 {
		t.Fatal("overlap after cancel", aCalls.Load())
	}
	cancel()
	select {
	case <-done:
		t.Fatal("shutdown abandoned running task")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("did not join")
	}
}
func TestLifecycleFairOffersAcrossRepeatedDueAccounts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reached := make(chan struct{}, 10)
	w := LifecycleWorker{Store: lifecycleSourceStub{[]droplets.LifecycleItem{{ID: "first", AccountID: "first"}, {ID: "later", AccountID: "later"}}}, Concurrency: 1, Interval: 5 * time.Millisecond,
		Handler: lifecycleHandlerFunc(func(ctx context.Context, i droplets.LifecycleItem) error {
			if i.ID == "later" {
				select {
				case reached <- struct{}{}:
				default:
				}
			}
			return nil
		})}
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatal("oldest due monopolized lane")
	}
	cancel()
	<-done
}
