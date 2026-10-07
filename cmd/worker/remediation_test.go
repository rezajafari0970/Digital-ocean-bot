package main

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"sync"
	"testing"
	"time"
)

func TestCapacityBatchTracksAcquisitionAcrossBoundedWaves(t *testing.T) {
	var mu sync.Mutex
	now := time.Unix(1000, 0)
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	registry := supervision.New(clock)
	policy, _ := modulePolicy("capacity-fill")
	ctx, err := registry.Register(context.Background(), "capacity-fill", policy)
	if err != nil {
		t.Fatal(err)
	}
	root, finish, err := supervision.Begin(ctx, "work", 0)
	if err != nil {
		t.Fatal(err)
	}
	budget := worker.NewWorkBudget(1)
	calls := 0
	err = worker.RunBoundedBatch(root, 4, 1, budget.Do, func(c context.Context, _ int) error {
		calls++
		if registry.Snapshot().Modules[0].Active < 2 {
			return errors.New("acquisition lacks admitted task")
		}
		mu.Lock()
		now = now.Add(8 * time.Second)
		mu.Unlock()
		return registry.Check()
	})
	finish()
	if err != nil || calls != 4 || !registry.Snapshot().Healthy {
		t.Fatal(calls, err, registry.Snapshot())
	}
}
func TestAsyncScannerCannotBeRefreshedByChildActivity(t *testing.T) {
	now := time.Unix(1000, 0)
	r := supervision.New(func() time.Time { return now })
	p, _ := modulePolicy("recovery")
	ctx, err := r.Register(context.Background(), "recovery", p)
	if err != nil {
		t.Fatal(err)
	}
	supervision.Pulse(ctx)
	task, done, err := supervision.Begin(ctx, "work", 0)
	if err != nil {
		t.Fatal(err)
	}
	long, finish, err := supervision.Begin(task, "execute", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	supervision.Idle(ctx)
	now = now.Add(50 * time.Second)
	supervision.Pulse(long)
	endWait := supervision.Waiting(task)
	endWait()
	now = now.Add(21 * time.Second)
	if err = r.Check(); !errors.Is(err, supervision.ErrStalled) || r.Snapshot().Healthy {
		t.Fatal("scanner stall hidden", err, r.Snapshot())
	}
	finish()
	done()
}
func TestAsyncHealthyScannerAllowsDeclaredLongChild(t *testing.T) {
	now := time.Unix(1000, 0)
	r := supervision.New(func() time.Time { return now })
	p, _ := modulePolicy("recovery")
	ctx, _ := r.Register(context.Background(), "recovery", p)
	task, done, _ := supervision.Begin(ctx, "work", 0)
	defer done()
	_, finish, _ := supervision.Begin(task, "execute", 15*time.Minute)
	defer finish()
	for i := 0; i < 20; i++ {
		supervision.Pulse(ctx)
		supervision.Idle(ctx)
		now = now.Add(10 * time.Second)
		if err := r.Check(); err != nil {
			t.Fatal(err)
		}
	}
}
