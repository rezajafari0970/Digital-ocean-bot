package testharness

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"testing"
)

func TestFailureInjectionEveryWorkflowStage(t *testing.T) {
	for _, stage := range []string{"wait_resource", "provision", "database", "panel", "clients", "traffic"} {
		t.Run(stage, func(t *testing.T) {
			store := &Store{}
			steps := NewSteps(FailurePlan{Step: stage, Times: 1})
			engine := workflow.Engine{Store: store, Steps: steps}
			req := workflow.Request{AccountID: "a", ProfileID: "p"}
			if _, err := engine.Run(context.Background(), req); err != ErrInjected {
				t.Fatalf("expected failure got %v", err)
			}
			d, err := engine.Run(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if d.State != workflow.Ready {
				t.Fatalf("state=%s", d.State)
			}
			if steps.ProviderCreates != 1 {
				t.Fatalf("duplicate create at %s: %d", stage, steps.ProviderCreates)
			}
		})
	}
}

func TestRepeatedRunAfterReadyIsNoop(t *testing.T) {
	store := &Store{}
	steps := NewSteps(FailurePlan{})
	engine := workflow.Engine{Store: store, Steps: steps}
	req := workflow.Request{AccountID: "a", ProfileID: "p"}
	if _, err := engine.Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	before := len(store.Events)
	if _, err := engine.Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(store.Events) != before {
		t.Fatal("ready workflow emitted new events")
	}
	if steps.ProviderCreates != 1 {
		t.Fatal("duplicate create")
	}
}
