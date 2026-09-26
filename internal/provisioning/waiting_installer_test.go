package provisioning

import (
	"context"
	"errors"
	"testing"
)

func TestPlaceholderInstallerPausesBeforeExecute(t *testing.T) {
	store := &memStore{}
	scripts := &scriptExecStub{}
	e := Engine{Store: store, Secrets: secretStub{}, SSH: &sshStub{}, Scripts: scripts}
	r, err := e.Execute(context.Background(), Target{AccountID: "a", DropletID: "d", KeySecretRef: "ssh"}, Plan{Scripts: []ScriptStep{{Name: "bootstrap", Category: "bootstrap", Execute: "prep", MaxAttempts: 3}, {Name: "panel", Category: "install", Execute: "true", MaxAttempts: 3}, {Name: "verify", Category: "verify", Execute: "check", MaxAttempts: 3}}})
	if !errors.Is(err, ErrInstallerNotConfigured) {
		t.Fatalf("err=%v", err)
	}
	if r.State != WaitingInstaller || r.CurrentStep != "panel" {
		t.Fatalf("state=%s step=%s", r.State, r.CurrentStep)
	}
	if len(scripts.calls) != 1 || scripts.calls[0] != "bootstrap" {
		t.Fatalf("calls=%v", scripts.calls)
	}
}
func TestHistoricalVerifyReconcilesToWaitingInstaller(t *testing.T) {
	store := &memStore{exists: true, r: Run{ID: "1", AccountID: "a", DropletID: "d", State: Verifying, CurrentStep: "verify"}}
	scripts := &scriptExecStub{}
	e := Engine{Store: store, Secrets: secretStub{}, SSH: &sshStub{}, Scripts: scripts}
	r, err := e.Execute(context.Background(), Target{AccountID: "a", DropletID: "d", KeySecretRef: "ssh"}, Plan{Scripts: []ScriptStep{{Name: "bootstrap", Category: "bootstrap", Execute: "prep", MaxAttempts: 3}, {Name: "panel", Category: "install", Execute: "true", MaxAttempts: 3}, {Name: "verify", Category: "verify", Execute: "check", MaxAttempts: 3}}})
	if !errors.Is(err, ErrInstallerNotConfigured) {
		t.Fatalf("err=%v", err)
	}
	if r.State != WaitingInstaller || r.CurrentStep != "panel" {
		t.Fatalf("state=%s step=%s", r.State, r.CurrentStep)
	}
	if len(scripts.calls) != 0 {
		t.Fatalf("historical commands reran: %v", scripts.calls)
	}
}
