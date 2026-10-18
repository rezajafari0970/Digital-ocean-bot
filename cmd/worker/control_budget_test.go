package main

import (
	"context"
	"testing"
	"time"
)

func TestControlBudgetReservesProviderProgress(t *testing.T) {
	parent, bulk, provider, _ := newControlWorkBudgets()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hold := make(chan struct{})
	started := make(chan struct{}, 3)
	done := make(chan struct{}, 4)
	job := func(ctx context.Context) error { started <- struct{}{}; <-hold; return nil }
	go func() { _ = bulk.Do(ctx, job); done <- struct{}{} }()
	<-started
	// A second bulk job cannot occupy either reserved position.
	go func() { _ = bulk.Do(ctx, job); done <- struct{}{} }()
	for range 2 {
		go func() { _ = provider.Do(ctx, job); done <- struct{}{} }()
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(hold)
			t.Fatal("provider starved behind bulk control")
		}
	}
	if parent.Snapshot()["active"] != 3 || provider.Snapshot()["admitted"] != 2 || bulk.Snapshot()["admitted"] != 1 {
		close(hold)
		t.Fatal("shared limits changed", parent.Snapshot(), provider.Snapshot(), bulk.Snapshot())
	}
	cancel()
	close(hold)
	for range 4 {
		<-done
	}
	if parent.Snapshot()["active"] != 0 {
		t.Fatal("permit leaked")
	}
}

func TestControlSchedulerIndependentFromBulk(t *testing.T) {
	parent, bulk, provider, identity := newControlWorkBudgets()
	scheduler := newSchedulerWorkBudget(parent)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	held := make(chan struct{}, 5)
	release := make(chan struct{})
	done := make(chan struct{}, 6)
	job := func(context.Context) error { held <- struct{}{}; <-release; return nil }
	launch := func(admit func(context.Context, func(context.Context) error) error) {
		go func() { _ = admit(ctx, job); done <- struct{}{} }()
	}
	launch(bulk.Do)
	launch(provider.Do)
	launch(provider.Do)
	launch(identity.Do)
	for range 4 {
		select {
		case <-held:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("reserved owner did not start")
		}
	}
	launch(bulk.Do)
	launch(scheduler.Do)
	select {
	case <-held:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("scheduler blocked behind bulk recovery")
	}
	if parent.Snapshot()["active"] != 5 || scheduler.Snapshot()["admitted"] != 1 || bulk.Snapshot()["admitted"] != 1 {
		t.Error("scheduler or bulk bounds changed")
	}
	cancel()
	close(release)
	for range 6 {
		<-done
	}
	if parent.Snapshot()["active"] != 0 {
		t.Fatal("permit leaked")
	}
}
