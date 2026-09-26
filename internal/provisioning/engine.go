package provisioning

import (
	"context"
	"errors"
	"time"
)

var ErrInvalidPlan = errors.New("invalid provision plan")
var ErrInterruptedUnsafe = errors.New("previous worker stopped during non-reconcilable script")
var ErrInstallerNotConfigured = errors.New("installer not configured")

type SecretReader interface {
	Get(context.Context, string, string) ([]byte, error)
}
type CommandRunner interface {
	Wait(context.Context, Target, []byte) error
	Run(context.Context, Target, []byte, string) (string, error)
}
type DetailedCommandRunner interface {
	RunDetailed(context.Context, Target, []byte, string) (CommandResult, error)
}
type ReadinessChecker interface {
	Collect(context.Context, string, Target, []byte, StageObserver) (ReadinessSnapshot, error)
}
type ObservableWaiter interface {
	WaitObserved(context.Context, Target, []byte, ProbeObserver) error
}
type StagedWaiter interface {
	WaitStages(context.Context, Target, []byte, ProbeObserver, StageObserver) error
}
type StagedCommandRunner interface {
	RunDetailedObserved(context.Context, Target, []byte, string, StageObserver) (CommandResult, error)
}
type Plan struct {
	Bootstrap    string       `json:"bootstrap,omitempty"`
	InstallPanel string       `json:"install_panel,omitempty"`
	Verify       string       `json:"verify,omitempty"`
	Scripts      []ScriptStep `json:"scripts,omitempty"`
}
type Engine struct {
	Store     Store
	Secrets   SecretReader
	SSH       CommandRunner
	Events    EventRecorder
	Scripts   ScriptExecutor
	Readiness ReadinessChecker
}

func (e Engine) Execute(ctx context.Context, target Target, plan Plan) (Run, error) {
	scripts := plan.Scripts
	if len(scripts) == 0 {
		if plan.Bootstrap == "" || plan.InstallPanel == "" || plan.Verify == "" {
			return Run{}, ErrInvalidPlan
		}
		scripts = LegacyScriptPlan(plan).Steps
	}
	if err := validateScriptPlan(scripts); err != nil {
		return Run{}, err
	}
	run, fresh, err := e.Store.Reserve(ctx, Run{AccountID: target.AccountID, DropletID: target.DropletID, State: Pending, CurrentStep: "ssh"})
	if err != nil {
		return Run{}, err
	}
	if !fresh && run.State == Completed {
		return run, nil
	}
	_ = fresh // retry ownership is per inner step, not the legacy run attempt counter
	key, err := e.Secrets.Get(ctx, target.AccountID, target.KeySecretRef)
	if err != nil {
		return run, err
	}
	defer wipe(key)
	type runtimeStep struct {
		name      string
		state     State
		script    *ScriptStep
		readiness bool
	}
	steps := []runtimeStep{{name: "ssh", state: WaitingSSH}}
	if e.Readiness != nil {
		steps = append(steps, runtimeStep{name: "readiness", state: CheckingReadiness, readiness: true})
	}
	for i := range scripts {
		steps = append(steps, runtimeStep{name: scripts[i].Name, state: stateForScript(scripts[i]), script: &scripts[i]})
	}
	order := map[string]int{"ssh": 0, "done": len(steps)}
	for i := 1; i < len(steps); i++ {
		order[steps[i].name] = i
	}
	// Historical runs may already be past a placeholder installer and stuck in
	// verify. Reconcile them back to an explicit paused installer state.
	for i := 1; i < len(steps); i++ {
		if steps[i].script != nil && IsInstallerPlaceholder(*steps[i].script) && order[run.CurrentStep] > i {
			run.State = WaitingInstaller
			run.CurrentStep = steps[i].name
			run.LastError = ErrInstallerNotConfigured.Error()
			run.NextRetryAt = nil
			_ = e.Store.Update(ctx, run)
			if e.Events != nil {
				diag := ClassifyError(ErrInstallerNotConfigured)
				_ = e.Events.Event(ctx, Event{RunID: run.ID, Step: steps[i].name, State: "WAITING_INSTALLER", Diagnostic: diag, Retryable: false})
			}
			return run, ErrInstallerNotConfigured
		}
	}
	for idx, step := range steps {
		if order[run.CurrentStep] > order[step.name] {
			continue
		}
		if step.script != nil && IsInstallerPlaceholder(*step.script) {
			run.State = WaitingInstaller
			run.CurrentStep = step.name
			run.LastError = ErrInstallerNotConfigured.Error()
			run.NextRetryAt = nil
			_ = e.Store.Update(ctx, run)
			if e.Events != nil {
				diag := ClassifyError(ErrInstallerNotConfigured)
				_ = e.Events.Event(ctx, Event{RunID: run.ID, Step: step.name, State: "WAITING_INSTALLER", Diagnostic: diag, Retryable: false})
			}
			return run, ErrInstallerNotConfigured
		}
		maxAttempts := DefaultStepPolicies["ssh"].MaxAttempts
		if step.readiness {
			maxAttempts = DefaultStepPolicies["readiness"].MaxAttempts
		}
		if step.script != nil {
			maxAttempts = step.script.MaxAttempts
		}
		interrupted, interruptErr := e.Store.StepInterrupted(ctx, run.ID, step.name)
		if interruptErr != nil {
			return run, interruptErr
		}
		if interrupted && step.script != nil && step.script.Execute != "" && step.script.Precheck == "" {
			run.State = Failed
			run.LastError = ErrInterruptedUnsafe.Error()
			_ = e.Store.FinishStep(ctx, run.ID, step.name, ErrInterruptedUnsafe, true)
			_ = e.Store.Update(ctx, run)
			if e.Events != nil {
				diag := ClassifyError(ErrInterruptedUnsafe)
				_ = e.Events.Event(ctx, Event{RunID: run.ID, Step: step.name, State: "FAILED", Diagnostic: diag, Retryable: false, Metadata: map[string]any{"reason": "interrupted_without_precheck"}})
			}
			return run, errors.Join(ErrStepTerminal, ErrInterruptedUnsafe)
		}
		stepAttempt, beginErr := e.Store.BeginStep(ctx, run.ID, step.name, maxAttempts)
		if beginErr != nil {
			if errors.Is(beginErr, ErrStepRetryDeferred) {
				return run, beginErr
			}
			run.State = Failed
			run.LastError = beginErr.Error()
			_ = e.Store.Update(ctx, run)
			return run, beginErr
		}
		run.State = step.state
		run.CurrentStep = step.name
		run.Attempt++
		if err := e.Store.Update(ctx, run); err != nil {
			return run, err
		}
		started := time.Now()
		if e.Events != nil {
			_ = e.Events.Event(ctx, Event{RunID: run.ID, Step: step.name, State: "RUNNING", Attempt: stepAttempt, Metadata: map[string]any{"host": target.Host, "port": target.Port, "user": target.User}})
		}
		var result CommandResult
		stageObserver := func(o StageObservation) {
			if e.Events == nil {
				return
			}
			state := "STAGE_OK"
			diag := Diagnostic{}
			if o.Err != nil {
				state = "STAGE_FAILED"
				diag = ClassifyError(o.Err)
			}
			_ = e.Events.Event(ctx, Event{RunID: run.ID, Step: step.name, Substep: string(o.Stage), State: state, Attempt: stepAttempt, Diagnostic: diag, Duration: o.Duration, Retryable: o.Err != nil})
		}
		if step.readiness {
			_, err = e.Readiness.Collect(ctx, run.ID, target, key, stageObserver)
		} else if step.name == "ssh" {
			if staged, ok := e.SSH.(StagedWaiter); ok {
				probe := 0
				err = staged.WaitStages(ctx, target, key, func(probeErr error, dur time.Duration) {
					probe++
					if e.Events == nil {
						return
					}
					state := "PROBE_OK"
					diag := Diagnostic{}
					if probeErr != nil {
						state = "PROBE_FAILED"
						diag = ClassifyError(probeErr)
					}
					_ = e.Events.Event(ctx, Event{RunID: run.ID, Step: step.name, Substep: "probe", State: state, Attempt: stepAttempt, Diagnostic: diag, Duration: dur, Retryable: probeErr != nil, Metadata: map[string]any{"probe": probe}})
				}, stageObserver)
			} else if observable, ok := e.SSH.(ObservableWaiter); ok {
				probe := 0
				err = observable.WaitObserved(ctx, target, key, func(probeErr error, dur time.Duration) {
					probe++
					if e.Events == nil {
						return
					}
					if probeErr != nil {
						diag := ClassifyError(probeErr)
						_ = e.Events.Event(ctx, Event{RunID: run.ID, Step: step.name, Substep: "probe", State: "PROBE_FAILED", Attempt: stepAttempt, Diagnostic: diag, Duration: dur, Retryable: true, Metadata: map[string]any{"probe": probe}})
						return
					}
					_ = e.Events.Event(ctx, Event{RunID: run.ID, Step: step.name, Substep: "probe", State: "PROBE_OK", Attempt: stepAttempt, Duration: dur, Metadata: map[string]any{"probe": probe}})
				})
			} else {
				err = e.SSH.Wait(ctx, target, key)
			}
		} else if e.Scripts != nil && step.script != nil {
			script := *step.script
			err = e.Scripts.RunScript(ctx, target, key, script, func(p ScriptPhaseResult) {
				result = p.Result
				if e.Events == nil {
					return
				}
				state := "PHASE_OK"
				diag := Diagnostic{}
				if p.Skipped {
					state = "PHASE_SKIPPED"
				} else if p.Err != nil {
					state = "PHASE_FAILED"
					diag = ClassifyCommandFailure(p.Err, p.Result)
				}
				diag.StdoutTail = p.Result.Stdout
				diag.StderrTail = p.Result.Stderr
				diag.ExitCode = p.Result.ExitCode
				diag.Signal = p.Result.Signal
				phaseRetryable := p.Err != nil && DiagnosticRetryable(diag)
				if diag.Code == "COMMAND_OUTCOME_UNKNOWN" && script.Precheck != "" {
					phaseRetryable = true
				}
				_ = e.Events.Event(ctx, Event{RunID: run.ID, Step: step.name, Substep: p.Phase, State: state, Attempt: stepAttempt, Diagnostic: diag, Duration: p.Duration, Retryable: phaseRetryable, Metadata: map[string]any{"category": script.Category}})
			}, stageObserver)
		} else if staged, ok := e.SSH.(StagedCommandRunner); ok {
			command := ""
			if step.script != nil {
				command = step.script.Execute
			}
			result, err = staged.RunDetailedObserved(ctx, target, key, command, stageObserver)
		} else if detailed, ok := e.SSH.(DetailedCommandRunner); ok {
			command := ""
			if step.script != nil {
				command = step.script.Execute
			}
			result, err = detailed.RunDetailed(ctx, target, key, command)
		} else {
			var out string
			command := ""
			if step.script != nil {
				command = step.script.Execute
			}
			out, err = e.SSH.Run(ctx, target, key, command)
			result.Stdout = tailDiagnostic(out)
		}
		duration := time.Since(started)
		if err != nil {
			run.LastError = err.Error()
			diag := ClassifyCommandFailure(err, result)
			retryable := DiagnosticRetryable(diag)
			var readinessErr *ReadinessError
			if errors.As(err, &readinessErr) {
				retryable = readinessErr.Retryable
			}
			// A state-changing command interrupted after start has unknown outcome.
			// Automatic retry is allowed only when the immutable script defines an
			// idempotent precheck that can reconcile desired state first.
			if diag.Code == "COMMAND_OUTCOME_UNKNOWN" && step.script != nil && step.script.Precheck != "" {
				retryable = true
			}
			terminal := !retryable
			_ = e.Store.FinishStep(ctx, run.ID, step.name, err, terminal)
			if terminal {
				run.State = Failed
			}
			if e.Events != nil {
				state := "RETRY_WAIT"
				if terminal {
					state = "FAILED"
				}
				_ = e.Events.Event(ctx, Event{RunID: run.ID, Step: step.name, State: state, Attempt: stepAttempt, Diagnostic: diag, Duration: duration, Retryable: !terminal})
			}
			_ = e.Store.Update(ctx, run)
			if terminal {
				return run, errors.Join(ErrStepTerminal, err)
			}
			return run, err
		}
		_ = e.Store.FinishStep(ctx, run.ID, step.name, nil, false)
		if e.Events != nil {
			_ = e.Events.Event(ctx, Event{RunID: run.ID, Step: step.name, State: "COMPLETED", Attempt: stepAttempt, Duration: duration, Diagnostic: Diagnostic{StdoutTail: result.Stdout, StderrTail: result.Stderr, ExitCode: result.ExitCode, Signal: result.Signal}})
		}
		// Persist the checkpoint immediately. A killed worker can then resume
		// at the next step instead of repeating an already successful command.
		if idx+1 < len(steps) {
			run.CurrentStep = steps[idx+1].name
		} else {
			run.CurrentStep = "done"
		}
		run.LastError = ""
		run.NextRetryAt = nil
		if err := e.Store.Update(ctx, run); err != nil {
			return run, err
		}
	}
	run.State = Completed
	run.CurrentStep = "done"
	return run, e.Store.Update(ctx, run)
}

func stateForScript(s ScriptStep) State {
	switch s.Category {
	case "bootstrap":
		return Bootstrapping
	case "install", "panel":
		return InstallingPanel
	case "verify":
		return Verifying
	default:
		return RunningScript
	}
}
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
