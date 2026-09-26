package provisioning

import (
	"context"
	"errors"
	"testing"
)

type unknownOutcomeScripts struct{}

func (unknownOutcomeScripts) RunScript(_ context.Context, _ Target, _ []byte, _ ScriptStep, observe func(ScriptPhaseResult), _ StageObserver) error {
	err := errors.Join(ErrSSHCommand, ErrCommandOutcomeUnknown, context.DeadlineExceeded)
	if observe != nil {
		observe(ScriptPhaseResult{Phase: "execute", Err: err})
	}
	return err
}
func TestUnknownCommandOutcomeWithoutPrecheckFailsClosed(t *testing.T) {
	store := &memStore{}
	e := Engine{Store: store, Secrets: secretStub{}, SSH: &sshStub{}, Scripts: unknownOutcomeScripts{}}
	_, err := e.Execute(context.Background(), Target{AccountID: "a", DropletID: "d", KeySecretRef: "ssh"}, Plan{Scripts: []ScriptStep{{Name: "install", Execute: "do", MaxAttempts: 3}}})
	if !errors.Is(err, ErrStepTerminal) {
		t.Fatalf("err=%v", err)
	}
	if store.r.State != Failed {
		t.Fatalf("state=%s", store.r.State)
	}
}
func TestUnknownCommandOutcomeWithPrecheckRemainsRetryable(t *testing.T) {
	store := &memStore{}
	e := Engine{Store: store, Secrets: secretStub{}, SSH: &sshStub{}, Scripts: unknownOutcomeScripts{}}
	_, err := e.Execute(context.Background(), Target{AccountID: "a", DropletID: "d", KeySecretRef: "ssh"}, Plan{Scripts: []ScriptStep{{Name: "install", Precheck: "test -f /done", Execute: "do", MaxAttempts: 3}}})
	if err == nil || errors.Is(err, ErrStepTerminal) {
		t.Fatalf("err=%v", err)
	}
	if store.r.State == Failed {
		t.Fatalf("unexpected terminal state")
	}
}
