package worker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWeightedAdmissionKeepsSmallWorkAndCleanup(t *testing.T) {
	b := NewWeightedWorkBudget(4, 7)
	heavy := b.WithWeight(6)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { done <- heavy.Do(ctx, func(context.Context) error { close(started); <-release; return nil }) }()
	<-started
	small, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := b.WithWeight(1).Do(small, func(context.Context) error { return nil }); err != nil {
		close(release)
		t.Fatal("small observation blocked behind expensive owner", err)
	}
	queued, cancelQueued := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelQueued()
	if err := heavy.Do(queued, func(context.Context) error { t.Error("weighted limit exceeded"); return nil }); !errors.Is(err, context.DeadlineExceeded) {
		close(release)
		t.Fatal(err)
	}
	cancel()
	if got := b.Snapshot(); got["active"] != 1 || got["cost_active"] != 6 || got["waiting"] != 0 {
		close(release)
		t.Fatal("cancel released active ownership or leaked queued reservation", got)
	}
	close(release)
	<-done
	if got := b.Snapshot(); got["active"] != 0 || got["cost_active"] != 0 || got["waiting"] != 0 {
		t.Fatal("permit leaked", got)
	}
	if err := heavy.Do(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
}
