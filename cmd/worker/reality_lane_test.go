package main

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"sync"
	"testing"
	"time"
)

func TestBootstrapProgressesWhileBulkClassIsOccupied(t *testing.T) {
	parent := worker.NewWorkBudget(10)
	bulk := worker.NewWorkClass(parent, 2)
	_, _, maintenance := realityLane(parent, bulk, false)
	name, n, bootstrap := realityLane(parent, bulk, true)
	if name != "global-reality-bootstrap" || n != 1 {
		t.Fatal(name, n)
	}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var wg sync.WaitGroup
	defer func() {
		close(release)
		wg.Wait()
		if parent.Snapshot()["active"] != 0 {
			t.Error("budget leaked")
		}
	}()
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			maintenance(context.Background(), func(context.Context) error { entered <- struct{}{}; <-release; return nil })
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("bulk fixture failed")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	called := false
	e := bootstrap(ctx, func(context.Context) error {
		called = true
		if parent.Snapshot()["active"] != 3 {
			t.Error("bootstrap bypassed shared budget")
		}
		return nil
	})
	if e != nil || !called {
		t.Fatalf("bootstrap starved: %v", e)
	}
}
