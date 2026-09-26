package provisioning

import (
	"context"
	"errors"
	"testing"
)

type installerStateStub struct{ states []string }

func (s *installerStateStub) SetState(_ context.Context, _, state, _ string) error {
	s.states = append(s.states, state)
	return nil
}
func TestInstallerExecutorCompletes(t *testing.T) {
	store := &memStore{}
	scripts := &scriptExecStub{}
	states := &installerStateStub{}
	e := InstallerExecutor{Store: store, Scripts: scripts, States: states}
	r := ResolvedInstaller{Manifest: InstallerManifest{Name: "demo", Version: 1}, Steps: []ScriptStep{{Name: "install", Category: "install", Execute: "do", MaxAttempts: 2}}}
	ir := InstallerRun{ID: "ir", ProvisionRunID: "pr"}
	if err := e.Execute(context.Background(), ir, r, Target{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(states.states) != 2 || states.states[0] != "INSTALLING" || states.states[1] != "INSTALL_COMPLETE" {
		t.Fatalf("%v", states.states)
	}
}

type installerFailScripts struct{}

func (installerFailScripts) RunScript(_ context.Context, _ Target, _ []byte, _ ScriptStep, observe func(ScriptPhaseResult), _ StageObserver) error {
	err := errors.Join(ErrSSHCommand, errors.New("permission denied"))
	if observe != nil {
		code := 1
		observe(ScriptPhaseResult{Phase: "execute", Result: CommandResult{Stderr: "permission denied", ExitCode: &code}, Err: err})
	}
	return err
}
func TestInstallerTerminalFailureRequiresRollback(t *testing.T) {
	store := &memStore{}
	states := &installerStateStub{}
	e := InstallerExecutor{Store: store, Scripts: installerFailScripts{}, States: states}
	r := ResolvedInstaller{Manifest: InstallerManifest{Name: "demo", Version: 1}, Steps: []ScriptStep{{Name: "install", Execute: "do", MaxAttempts: 2}}, Rollback: []ScriptStep{{Name: "undo", Execute: "undo", MaxAttempts: 1}}}
	err := e.Execute(context.Background(), InstallerRun{ID: "ir", ProvisionRunID: "pr"}, r, Target{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := states.states[len(states.states)-1]; got != "ROLLBACK_REQUIRED" {
		t.Fatalf("state=%s", got)
	}
}
