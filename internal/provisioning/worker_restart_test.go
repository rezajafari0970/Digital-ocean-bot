package provisioning

import (
	"context"
	"errors"
	"testing"
)

func TestRestartAfterKilledWorkerFailsClosedWithoutPrecheck(t *testing.T) {
	store := &memStore{exists: true, r: Run{ID: "1", AccountID: "a", DropletID: "d", State: RunningScript, CurrentStep: "install"}, interrupted: map[string]bool{"install": true}}
	scripts := &scriptExecStub{}
	e := Engine{Store: store, Secrets: secretStub{}, SSH: &sshStub{}, Scripts: scripts}
	_, err := e.Execute(context.Background(), Target{AccountID: "a", DropletID: "d", KeySecretRef: "ssh"}, Plan{Scripts: []ScriptStep{{Name: "install", Execute: "apt install x", MaxAttempts: 3}}})
	if !errors.Is(err, ErrInterruptedUnsafe) {
		t.Fatalf("err=%v", err)
	}
	if len(scripts.calls) != 0 {
		t.Fatalf("unsafe execute ran: %v", scripts.calls)
	}
}
func TestRestartAfterKilledWorkerReconcilesWhenPrecheckExists(t *testing.T) {
	store := &memStore{exists: true, r: Run{ID: "1", AccountID: "a", DropletID: "d", State: RunningScript, CurrentStep: "install"}, interrupted: map[string]bool{"install": true}}
	scripts := &scriptExecStub{}
	e := Engine{Store: store, Secrets: secretStub{}, SSH: &sshStub{}, Scripts: scripts}
	r, err := e.Execute(context.Background(), Target{AccountID: "a", DropletID: "d", KeySecretRef: "ssh"}, Plan{Scripts: []ScriptStep{{Name: "install", Precheck: "dpkg -s x", Execute: "apt install x", MaxAttempts: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	if r.State != Completed {
		t.Fatalf("state=%s", r.State)
	}
	if len(scripts.calls) != 1 {
		t.Fatalf("calls=%v", scripts.calls)
	}
}
