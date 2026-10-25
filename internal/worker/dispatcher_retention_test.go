package worker

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// A long-lived dispatcher must retain only accounts with unfinished work,
// regardless of how many distinct account identities it has seen.
func TestDispatcherForgetsCompletedAccounts(t *testing.T) {
	d := NewDispatcher(4, 2)
	for i := 0; i < 512; i++ {
		key := fmt.Sprintf("job-%d", i)
		account := fmt.Sprintf("account-%d", i)
		if !d.Submit(context.Background(), key, account, func() {}) {
			t.Fatal("idle dispatcher rejected", i)
		}
		d.Wait()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.accounts) != 0 || len(d.active) != 0 || len(d.started) != 0 {
		t.Fatalf("completed work retained: accounts=%d active=%d started=%d",
			len(d.accounts), len(d.active), len(d.started))
	}
}

func TestDispatcherCancellationKeepsExecutingAccountOwned(t *testing.T) {
	d := NewDispatcher(2, 1)
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	started := make(chan struct{})
	if !d.Submit(ctx, "old", "account", func() { close(started); <-release }) {
		t.Fatal("first submission rejected")
	}
	defer func() { close(release); d.Wait() }()
	<-started
	cancel()
	if d.Submit(context.Background(), "replacement", "account", func() { t.Error("overlapped unfinished handler") }) {
		t.Fatal("cancellation released an executing account")
	}
	if !d.Active("old") {
		t.Fatal("unfinished handler identity disappeared")
	}
}

func TestDispatcherAccountCountSurvivesSiblingCompletion(t *testing.T) {
	d := NewDispatcher(3, 2)
	releaseFirst, releaseOthers := make(chan struct{}), make(chan struct{})
	defer func() { close(releaseOthers); d.Wait() }()
	if !d.Submit(context.Background(), "first", "account", func() { <-releaseFirst }) {
		t.Fatal("first submission rejected")
	}
	if !d.Submit(context.Background(), "second", "account", func() { <-releaseOthers }) {
		close(releaseFirst)
		t.Fatal("second submission rejected")
	}
	close(releaseFirst)
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for d.Active("first") {
		select {
		case <-deadline.C:
			t.Fatal("first handler did not complete")
		case <-tick.C:
		}
	}
	if !d.Submit(context.Background(), "third", "account", func() { <-releaseOthers }) {
		t.Fatal("completed sibling did not release exactly one slot")
	}
	if d.Submit(context.Background(), "fourth", "account", func() { t.Error("account limit bypassed") }) {
		t.Fatal("remaining siblings lost their account reservation")
	}
}
