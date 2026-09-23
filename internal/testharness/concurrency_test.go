package testharness

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"sync"
	"testing"
)

func TestConcurrentReadyRunsDoNotDuplicateCreate(t *testing.T) {
	store := &Store{}
	steps := NewSteps(FailurePlan{})
	engine := workflow.Engine{Store: store, Steps: steps, RunLock: &store.runMu}
	req := workflow.Request{AccountID: "a", ProfileID: "p"}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := engine.Run(context.Background(), req); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if steps.ProviderCreates != 1 {
		t.Fatalf("duplicate creates=%d", steps.ProviderCreates)
	}
}
