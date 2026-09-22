package testharness

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"testing"
)

func TestEndToEndHappyPath(t *testing.T) {
	store := &Store{}
	steps := NewSteps(FailurePlan{})
	engine := workflow.Engine{Store: store, Steps: steps}
	d, err := engine.Run(context.Background(), workflow.Request{AccountID: "a", ProfileID: "p", ClientCount: 10, InboundID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if d.State != workflow.Ready {
		t.Fatalf("state=%s", d.State)
	}
	if steps.ProviderCreates != 1 {
		t.Fatalf("creates=%d", steps.ProviderCreates)
	}
	for _, name := range []string{"create", "wait_resource", "provision", "database", "panel", "clients", "traffic"} {
		if steps.Calls[name] != 1 {
			t.Fatalf("%s calls=%d", name, steps.Calls[name])
		}
	}
}

func TestFailureResumeDoesNotRepeatCompletedSteps(t *testing.T) {
	store := &Store{}
	steps := NewSteps(FailurePlan{Step: "database", Times: 1})
	engine := workflow.Engine{Store: store, Steps: steps}
	req := workflow.Request{AccountID: "a", ProfileID: "p"}
	_, err := engine.Run(context.Background(), req)
	if err != ErrInjected {
		t.Fatalf("expected injected failure got %v", err)
	}
	if steps.ProviderCreates != 1 {
		t.Fatal("create count")
	}
	_, err = engine.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if steps.ProviderCreates != 1 || steps.Calls["provision"] != 1 || steps.Calls["database"] != 2 {
		t.Fatalf("unexpected replay: %#v", steps.Calls)
	}
}
