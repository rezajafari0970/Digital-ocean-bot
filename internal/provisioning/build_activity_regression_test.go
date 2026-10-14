package provisioning

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type precheckExitStub struct {
	result CommandResult
	calls  []string
}

func (s *precheckExitStub) RunDetailedObserved(_ context.Context, _ Target, _ []byte, cmd string, obs StageObserver) (CommandResult, error) {
	s.calls = append(s.calls, cmd)
	if cmd != "check" {
		return CommandResult{}, nil
	}
	err := fmt.Errorf("%w: predicate failed", ErrSSHCommand)
	if obs != nil {
		obs(StageObservation{Stage: StageCommand, Err: err})
	}
	return s.result, err
}
func TestPrecheckOnlyFalsePredicateAllowsExecution(t *testing.T) {
	for _, code := range []int{1, 2, 126, 127, -1} {
		for _, signal := range []string{"", "TERM"} {
			t.Run(fmt.Sprintf("exit%d-%s", code, signal), func(t *testing.T) {
				var exit *int
				if code >= 0 {
					n := code
					exit = &n
				}
				ssh := &precheckExitStub{result: CommandResult{ExitCode: exit, Signal: signal}}
				failed := 0
				err := (SSHScriptRunner{SSH: ssh}).RunScript(context.Background(), Target{}, nil, ScriptStep{Precheck: "check", Execute: "install", Verify: "verify"}, nil, func(o StageObservation) {
					if o.Err != nil {
						failed++
					}
				})
				miss := code == 1 && signal == ""
				if miss {
					if err != nil || len(ssh.calls) != 3 || failed != 0 {
						t.Fatal(ssh.calls, err, failed)
					}
				} else if err == nil || len(ssh.calls) != 1 || failed != 1 {
					t.Fatal("unsafe predicate permitted execution", ssh.calls, err, failed)
				}
			})
		}
	}
}

type handoffFaultStore struct {
	memStore
	fail bool
}

func (s *handoffFaultStore) Update(ctx context.Context, r Run) error {
	if s.fail && r.State == WaitingInstaller {
		return errors.New("handoff checkpoint unavailable")
	}
	return s.memStore.Update(ctx, r)
}
func TestSelectedInstallerHandoffIsInformationalAndDurable(t *testing.T) {
	for _, old := range []string{"panel", "verify"} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-fault%v", old, fail), func(t *testing.T) {
				store := &handoffFaultStore{memStore: memStore{exists: true, r: Run{ID: "r", CurrentStep: old}}, fail: fail}
				events := &eventRecorderStub{}
				scripts := &scriptExecStub{}
				run, err := (Engine{Store: store, Secrets: secretStub{}, SSH: &sshStub{}, Scripts: scripts, Events: events}).Execute(context.Background(), Target{}, Plan{InstallerHandoff: true, Scripts: []ScriptStep{{Name: "panel", Category: "install", Execute: "true"}, {Name: "verify", Category: "verify", Execute: "check"}}})
				if fail {
					if err == nil || errors.Is(err, ErrInstallerHandoff) || len(events.events) != 0 {
						t.Fatal(run, err, events.events)
					}
					return
				}
				if !errors.Is(err, ErrInstallerHandoff) || run.LastError != "" || store.r.State != WaitingInstaller || len(scripts.calls) != 0 || len(events.events) != 1 || events.events[0].Diagnostic.Fingerprint != "" {
					t.Fatal(run, err, events.events)
				}
			})
		}
	}
}
func TestInstallerCheckpointFailureNeverAdvances(t *testing.T) {
	fault := errors.New("durable step checkpoint unavailable")
	for _, rollback := range []bool{false, true} {
		t.Run(fmt.Sprint(rollback), func(t *testing.T) {
			store := &checkpointStore{completed: map[string]bool{}, finishErr: fault}
			states := &installerStateStub{}
			scripts := &scriptExecStub{}
			executor := InstallerExecutor{Store: store, States: states, Scripts: scripts}
			steps := []ScriptStep{{Name: "first", Execute: "effect", Precheck: "check", MaxAttempts: 2}, {Name: "second", Execute: "next", Precheck: "check", MaxAttempts: 2}}
			resolved := ResolvedInstaller{Manifest: InstallerManifest{Name: "fixture", Version: 1}, Steps: steps, Rollback: steps}
			var err error
			if rollback {
				err = executor.Rollback(context.Background(), InstallerRun{ID: "ir", ProvisionRunID: "pr"}, resolved, Target{}, nil)
			} else {
				err = executor.Execute(context.Background(), InstallerRun{ID: "ir", ProvisionRunID: "pr"}, resolved, Target{}, nil)
			}
			if !errors.Is(err, fault) || len(scripts.calls) != 1 {
				t.Fatal(err, scripts.calls)
			}
			for _, state := range states.states {
				if state == "INSTALL_COMPLETE" || state == "ROLLED_BACK" {
					t.Fatal("false completion", states.states)
				}
			}
		})
	}
}
func TestInstallerFailureCheckpointErrorIsPreserved(t *testing.T) {
	fault := errors.New("failure checkpoint unavailable")
	store := &checkpointStore{completed: map[string]bool{}, finishErr: fault}
	states := &installerStateStub{}
	err := (InstallerExecutor{Store: store, States: states, Scripts: installerFailScripts{}}).Execute(context.Background(), InstallerRun{}, ResolvedInstaller{Steps: []ScriptStep{{Name: "install", Execute: "do", MaxAttempts: 1}}}, Target{}, nil)
	if !errors.Is(err, fault) || !errors.Is(err, ErrSSHCommand) || len(states.states) != 1 {
		t.Fatal(err, states.states)
	}
}

func TestTypedUnknownOutcomeCannotBecomeRetryableDisconnect(t *testing.T) {
	for _, text := range []string{"broken pipe", "Temporary failure resolving host", "process killed", "permission denied"} {
		err := errors.Join(ErrCommandOutcomeUnknown, errors.New(text))
		diagnostic := ClassifyCommandFailure(err, CommandResult{Stderr: text})
		if diagnostic.Code != "COMMAND_OUTCOME_UNKNOWN" || DiagnosticRetryable(diagnostic) {
			t.Fatal(diagnostic)
		}
	}
	for _, err := range []error{ErrHostKeyMismatch, ErrHostKeyVerifierMissing, ErrInterruptedUnsafe} {
		diagnostic := ClassifyCommandFailure(err, CommandResult{Stderr: "broken pipe"})
		if DiagnosticRetryable(diagnostic) {
			t.Fatal(diagnostic)
		}
	}
}

func TestProvisionDatabaseAdmissionFailureDoesNotDeclareTerminal(t *testing.T) {
	fault := errors.New("database admission unavailable")
	store := &checkpointStore{memStore: memStore{exists: true, r: Run{ID: "r", State: Pending, CurrentStep: "prepare"}}, completed: map[string]bool{}, beginErr: fault}
	ssh := &sshStub{}
	run, err := (Engine{Store: store, Secrets: secretStub{}, SSH: ssh}).Execute(context.Background(), Target{}, Plan{Scripts: []ScriptStep{{Name: "prepare", Category: "bootstrap", Execute: "prepare"}}})
	if !errors.Is(err, fault) || run.State == Failed || store.r.State == Failed || ssh.runs != 0 {
		t.Fatal(run, err, ssh.runs)
	}
	states := &installerStateStub{}
	err = (InstallerExecutor{Store: store, States: states, Scripts: &scriptExecStub{}}).Execute(context.Background(), InstallerRun{}, ResolvedInstaller{Steps: []ScriptStep{{Name: "prepare", Execute: "prepare"}}}, Target{}, nil)
	if !errors.Is(err, fault) || len(states.states) != 1 || states.states[0] != "INSTALLING" {
		t.Fatal(err, states.states)
	}
}
func TestInterruptedUnsafeRollbackNeverRepeatsSideEffect(t *testing.T) {
	store := &memStore{interrupted: map[string]bool{"installer-g1-demo-v1-rollback-undo": true}}
	scripts := &scriptExecStub{}
	states := &installerStateStub{}
	err := (InstallerExecutor{Store: store, Scripts: scripts, States: states}).Rollback(context.Background(), InstallerRun{Generation: 1}, ResolvedInstaller{Manifest: InstallerManifest{Name: "demo", Version: 1}, Rollback: []ScriptStep{{Name: "undo", Execute: "undo", MaxAttempts: 2}}}, Target{}, nil)
	if !errors.Is(err, ErrInterruptedUnsafe) || len(scripts.calls) != 0 || len(states.states) != 1 || states.states[0] != "FAILED" {
		t.Fatal(err, scripts.calls, states.states)
	}
}
func TestInvalidPrecheckIsNeverRecordedAsPredicateMiss(t *testing.T) {
	for _, installer := range []bool{false, true} {
		code := 127
		runner := &precheckExitStub{result: CommandResult{ExitCode: &code}}
		events := &eventRecorderStub{}
		scripts := SSHScriptRunner{SSH: runner}
		step := ScriptStep{Name: "prepare", Category: "bootstrap", Precheck: "check", Execute: "install", MaxAttempts: 1}
		if installer {
			_ = (InstallerExecutor{Store: &memStore{}, Scripts: scripts, Events: events, States: &installerStateStub{}}).Execute(context.Background(), InstallerRun{}, ResolvedInstaller{Steps: []ScriptStep{step}}, Target{}, nil)
		} else {
			_, _ = (Engine{Store: &memStore{}, Secrets: secretStub{}, SSH: &sshStub{}, Scripts: scripts, Events: events}).Execute(context.Background(), Target{}, Plan{Scripts: []ScriptStep{step}})
		}
		found := false
		for _, event := range events.events {
			if event.State == "PRECHECK_MISS" {
				t.Fatal(event)
			}
			if event.Substep == "precheck" {
				found = true
				if event.State != "PHASE_FAILED" || event.Diagnostic.Fingerprint == "" {
					t.Fatal(event)
				}
			}
		}
		if !found || len(runner.calls) != 1 {
			t.Fatal(installer, found, runner.calls)
		}
	}
}

type ambiguousInstallerSSH struct {
	desired                     bool
	executed, checked, verified int
}

func (s *ambiguousInstallerSSH) RunDetailedObserved(_ context.Context, _ Target, _ []byte, cmd string, _ StageObserver) (CommandResult, error) {
	switch cmd {
	case "check":
		s.checked++
		if !s.desired {
			one := 1
			return CommandResult{ExitCode: &one}, ErrSSHCommand
		}
	case "install":
		s.executed++
		s.desired = true
		return CommandResult{Stderr: "broken pipe"}, errors.Join(ErrCommandOutcomeUnknown, errors.New("broken pipe"))
	case "verify":
		s.verified++
	}
	zero := 0
	return CommandResult{ExitCode: &zero}, nil
}

type reconciliationStore struct {
	checkpointStore
	terminals []bool
}

func (s *reconciliationStore) FinishStep(ctx context.Context, run, step string, err error, terminal bool) error {
	s.terminals = append(s.terminals, terminal)
	return s.checkpointStore.FinishStep(ctx, run, step, err, terminal)
}
func TestInstallerUnknownOutcomeReconcilesWithoutRepeating(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		t.Run(fmt.Sprint(rollback), func(t *testing.T) {
			store := &reconciliationStore{checkpointStore: checkpointStore{completed: map[string]bool{}}}
			states := &installerStateStub{}
			ssh := &ambiguousInstallerSSH{}
			events := &eventRecorderStub{}
			executor := InstallerExecutor{Store: store, States: states, Scripts: SSHScriptRunner{SSH: ssh}, Events: events}
			step := ScriptStep{Name: "install", Precheck: "check", Execute: "install", Verify: "verify", MaxAttempts: 2}
			resolved := ResolvedInstaller{Manifest: InstallerManifest{Name: "fixture", Version: 1}, Steps: []ScriptStep{step}, Rollback: []ScriptStep{step}}
			run := func() error {
				if rollback {
					return executor.Rollback(context.Background(), InstallerRun{ID: "ir", ProvisionRunID: "pr"}, resolved, Target{}, nil)
				}
				return executor.Execute(context.Background(), InstallerRun{ID: "ir", ProvisionRunID: "pr"}, resolved, Target{}, nil)
			}
			if err := run(); !errors.Is(err, ErrCommandOutcomeUnknown) || len(store.terminals) != 1 || store.terminals[0] {
				t.Fatal(err, store.terminals)
			}
			expected := "INSTALLING"
			if rollback {
				expected = "ROLLBACK_REQUIRED"
			}
			if states.states[len(states.states)-1] != expected {
				t.Fatal(states.states)
			}
			if !rollback {
				found := false
				for _, event := range events.events {
					if event.Diagnostic.Code == "COMMAND_OUTCOME_UNKNOWN" {
						found = true
						if !event.Retryable {
							t.Fatal(event)
						}
					}
				}
				if !found {
					t.Fatal(events.events)
				}
			}
			if err := run(); err != nil {
				t.Fatal(err)
			}
			expected = "INSTALL_COMPLETE"
			if rollback {
				expected = "ROLLED_BACK"
			}
			if ssh.executed != 1 || ssh.checked != 2 || ssh.verified != 1 || states.states[len(states.states)-1] != expected {
				t.Fatal(ssh, states.states)
			}
		})
	}
}
func TestInstallerUnknownOutcomeWithoutReconciliationOrBudgetIsTerminal(t *testing.T) {
	for _, tc := range []struct {
		precheck string
		max      int
	}{{"", 3}, {"check", 1}} {
		store := &reconciliationStore{checkpointStore: checkpointStore{completed: map[string]bool{}}}
		states := &installerStateStub{}
		events := &eventRecorderStub{}
		err := (InstallerExecutor{Store: store, States: states, Events: events, Scripts: SSHScriptRunner{SSH: &ambiguousInstallerSSH{}}}).Execute(context.Background(), InstallerRun{}, ResolvedInstaller{Steps: []ScriptStep{{Name: "install", Execute: "install", Precheck: tc.precheck, MaxAttempts: tc.max}}}, Target{}, nil)
		if !errors.Is(err, ErrCommandOutcomeUnknown) || !store.terminals[0] || states.states[len(states.states)-1] != "FAILED" {
			t.Fatal(err, store.terminals, states.states)
		}
		for _, event := range events.events {
			if event.Diagnostic.Code == "COMMAND_OUTCOME_UNKNOWN" && event.Retryable {
				t.Fatal(event)
			}
		}
	}
}
