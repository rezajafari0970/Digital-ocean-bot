package main

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"testing"
	"time"
)

func TestProfileDeadlineBeginsAfterAdmission(t *testing.T) {
	budget := worker.NewWorkBudget(1)
	held, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- budget.Do(context.Background(), func(context.Context) error { close(held); <-release; return nil })
	}()
	<-held
	result := make(chan error, 1)
	started := make(chan time.Duration, 1)
	go func() {
		result <- runConfigProfileWork(context.Background(), budget.Do, 100*time.Millisecond, func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			if !ok {
				return errors.New("missing execution deadline")
			}
			started <- time.Until(deadline)
			return nil
		})
	}()
	select {
	case <-started:
		t.Fatal("ran before admission")
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-result:
		if e != nil {
			t.Fatal("queue consumed runtime budget", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("admitted work stalled")
	}
	if left := <-started; left < 75*time.Millisecond {
		t.Fatalf("execution budget shortened by queue wait: %s", left)
	}
}
func TestQueuedProfileHonorsWorkerCancellation(t *testing.T) {
	budget := worker.NewWorkBudget(1)
	held, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- budget.Do(context.Background(), func(context.Context) error { close(held); <-release; return nil })
	}()
	<-held
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	e := runConfigProfileWork(ctx, budget.Do, time.Second, func(context.Context) error { called = true; return nil })
	close(release)
	<-done
	if !errors.Is(e, context.Canceled) || called {
		t.Fatal("cancelled queued operation ran", e)
	}
}

func TestProfileModulesFitEveryParentBudget(t *testing.T) {
	for limit := 1; limit <= 10; limit++ {
		var modules worker.Modules
		addConfigProfileModules(&modules, nil, nil, worker.NewWorkBudget(limit))
	}
}
