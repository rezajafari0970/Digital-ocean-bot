package provisioning

import (
	"context"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"testing"
	"time"
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

type supervisedScriptProbe struct{ run func(context.Context) error }

func (s supervisedScriptProbe) RunDetailedObserved(c context.Context, _ Target, _ []byte, _ string, _ StageObserver) (CommandResult, error) {
	return CommandResult{}, s.run(c)
}
func TestScriptRunnerSupervisesDeclaredLongPhase(t *testing.T) {
	now := time.Now()
	registry := supervision.New(func() time.Time { return now })
	c, err := registry.Register(context.Background(), "recovery", supervision.Policy{Loop: time.Second, Work: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	c, finish, err := supervision.Begin(c, "work", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	probe := supervisedScriptProbe{run: func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("native timeout lost")
		}
		now = now.Add(10 * time.Minute)
		if err := registry.Check(); err != nil {
			t.Fatal("valid installer killed by short outer deadline", err)
		}
		if registry.Snapshot().Modules[0].Active != 2 {
			t.Fatal("script stage not independently watched")
		}
		return nil
	}}
	if err := (SSHScriptRunner{SSH: probe}).RunScript(c, Target{}, nil, ScriptStep{Execute: "install", Timeout: 15 * time.Minute}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := registry.Check(); err != nil {
		t.Fatal("outer deadline not resumed", err)
	}
	now = now.Add(2 * time.Second)
	if err := registry.Check(); !errors.Is(err, supervision.ErrStalled) {
		t.Fatal("outer work became unbounded", err)
	}
}
