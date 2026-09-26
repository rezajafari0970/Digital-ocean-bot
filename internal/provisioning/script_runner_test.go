package provisioning

import (
	"context"
	"fmt"
	"testing"
)

type stagedScriptStub struct {
	calls []string
	fail  map[string]bool
}

func (s *stagedScriptStub) RunDetailedObserved(_ context.Context, _ Target, _ []byte, cmd string, _ StageObserver) (CommandResult, error) {
	s.calls = append(s.calls, cmd)
	if s.fail[cmd] {
		code := 1
		return CommandResult{ExitCode: &code, Stderr: "not ready"}, fmt.Errorf("%w: exit 1", ErrSSHCommand)
	}
	code := 0
	return CommandResult{ExitCode: &code, Stdout: "ok"}, nil
}
func TestScriptRunnerPrecheckSkipsExecuteAndVerifies(t *testing.T) {
	stub := &stagedScriptStub{fail: map[string]bool{}}
	r := SSHScriptRunner{SSH: stub}
	err := r.RunScript(context.Background(), Target{}, []byte("k"), ScriptStep{Name: "x", Precheck: "check", Execute: "install", Verify: "verify"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 2 || stub.calls[0] != "check" || stub.calls[1] != "verify" {
		t.Fatalf("calls=%v", stub.calls)
	}
}
func TestScriptRunnerFailedPrecheckExecutesThenVerifies(t *testing.T) {
	stub := &stagedScriptStub{fail: map[string]bool{"check": true}}
	r := SSHScriptRunner{SSH: stub}
	err := r.RunScript(context.Background(), Target{}, []byte("k"), ScriptStep{Name: "x", Precheck: "check", Execute: "install", Verify: "verify"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 3 || stub.calls[1] != "install" || stub.calls[2] != "verify" {
		t.Fatalf("calls=%v", stub.calls)
	}
}
func TestScriptRunnerPrecheckInfrastructureFailureDoesNotExecute(t *testing.T) {
	stub := &stagedScriptStub{fail: map[string]bool{"check": true}}
	// Override the generic stub behavior with a runner that returns DNS stderr.
	dns := &dnsPrecheckStub{}
	r := SSHScriptRunner{SSH: dns}
	err := r.RunScript(context.Background(), Target{}, []byte("k"), ScriptStep{Name: "x", Precheck: "check", Execute: "install"}, nil, nil)
	if err == nil {
		t.Fatal("expected DNS failure")
	}
	if len(dns.calls) != 1 || dns.calls[0] != "check" {
		t.Fatalf("execute should not run: %v", dns.calls)
	}
	_ = stub
}

type dnsPrecheckStub struct{ calls []string }

func (s *dnsPrecheckStub) RunDetailedObserved(_ context.Context, _ Target, _ []byte, cmd string, _ StageObserver) (CommandResult, error) {
	s.calls = append(s.calls, cmd)
	code := 1
	return CommandResult{ExitCode: &code, Stderr: "Temporary failure resolving 'archive.ubuntu.com'"}, fmt.Errorf("%w: exit 1", ErrSSHCommand)
}
