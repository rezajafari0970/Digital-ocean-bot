package main

import (
	"context"
	"testing"
	"time"
)

func TestControlBudgetReservesProviderProgress(t *testing.T) {
	parent, bulk, provider := newControlWorkBudgets()
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
