package provisioning

import (
	"context"
	"testing"
)

type scriptExecStub struct {
	calls    []string
	failOnce string
	failed   bool
}

func (s *scriptExecStub) RunScript(_ context.Context, _ Target, _ []byte, step ScriptStep, _ func(ScriptPhaseResult), _ StageObserver) error {
	s.calls = append(s.calls, step.Name)
	if step.Name == s.failOnce && !s.failed {
		s.failed = true
		return ErrSSHCommand
	}
	return nil
}
func TestDynamicScriptPlanResumesAtFailedStep(t *testing.T) {
	store := &memStore{}
	ssh := &sshStub{}
	scripts := &scriptExecStub{failOnce: "step-2"}
	e := Engine{Store: store, Secrets: secretStub{}, SSH: ssh, Scripts: scripts}
	plan := Plan{Scripts: []ScriptStep{
		{Name: "step-1", Category: "script", Execute: "one", MaxAttempts: 3},
		{Name: "step-2", Category: "script", Execute: "two", MaxAttempts: 3},
		{Name: "step-3", Category: "script", Execute: "three", MaxAttempts: 3},
	}}
	target := Target{AccountID: "a", DropletID: "d", KeySecretRef: "ssh"}
	if _, err := e.Execute(context.Background(), target, plan); err == nil {
		t.Fatal("expected injected failure")
	}
	if store.r.CurrentStep != "step-2" {
		t.Fatalf("current=%s", store.r.CurrentStep)
	}
	// Fake store has no wall-clock backoff, so a fresh engine can resume immediately.
	e = Engine{Store: store, Secrets: secretStub{}, SSH: ssh, Scripts: scripts}
	r, err := e.Execute(context.Background(), target, plan)
	if err != nil {
		t.Fatal(err)
	}
	if r.State != Completed {
		t.Fatalf("state=%s", r.State)
	}
	want := []string{"step-1", "step-2", "step-2", "step-3"}
	if len(scripts.calls) != len(want) {
		t.Fatalf("calls=%v", scripts.calls)
	}
	for i := range want {
		if scripts.calls[i] != want[i] {
			t.Fatalf("calls=%v", scripts.calls)
		}
	}
}
