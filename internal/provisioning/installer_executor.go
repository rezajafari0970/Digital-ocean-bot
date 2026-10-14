package provisioning

import (
	"context"
	"errors"
	"fmt"
)

type InstallerStateStore interface {
	SetState(context.Context, string, string, string) error
}
type InstallerExecutor struct {
	Store   Store
	Events  EventRecorder
	Scripts ScriptExecutor
	States  InstallerStateStore
}

func (e InstallerExecutor) Execute(ctx context.Context, ir InstallerRun, resolved ResolvedInstaller, target Target, key []byte) error {
	if err := e.States.SetState(ctx, ir.ID, "INSTALLING", ""); err != nil {
		return err
	}
	verifying := false
	for _, step := range resolved.Steps {
		if !verifying && (step.Category == "verify" || step.Category == "health") {
			if err := e.States.SetState(ctx, ir.ID, "VERIFYING", ""); err != nil {
				return err
			}
			verifying = true
		}
		name := installerStepName(resolved.Manifest, ir.Generation, step.Name)
		interrupted, ierr := e.Store.StepInterrupted(ctx, ir.ProvisionRunID, name)
		if ierr != nil {
			return ierr
		}
		if interrupted && step.Execute != "" && step.Precheck == "" {
			err := ErrInterruptedUnsafe
			if persistErr := e.Store.FinishStep(ctx, ir.ProvisionRunID, name, err, true); persistErr != nil {
				return errors.Join(err, persistErr)
			}
			state := "FAILED"
			if len(resolved.Rollback) > 0 {
				state = "ROLLBACK_REQUIRED"
			}
			return errors.Join(err, e.States.SetState(ctx, ir.ID, state, err.Error()))
		}
		if cs, ok := e.Store.(StepCompletionStore); ok {
			done, derr := cs.StepCompleted(ctx, ir.ProvisionRunID, name)
			if derr != nil {
				return derr
			}
			if done {
				continue
			}
		}
		attempt, err := e.Store.BeginStep(ctx, ir.ProvisionRunID, name, step.MaxAttempts)
		if err != nil {
			if !errors.Is(err, ErrStepTerminal) && !errors.Is(err, ErrStepRetryLimit) {
				return err
			}
			state := "FAILED"
			if len(resolved.Rollback) > 0 {
				state = "ROLLBACK_REQUIRED"
			}
			return errors.Join(err, e.States.SetState(ctx, ir.ID, state, err.Error()))
		}
		var last CommandResult
		err = e.Scripts.RunScript(ctx, target, key, step, func(p ScriptPhaseResult) {
			last = p.Result
			if e.Events != nil {
				state := "PHASE_OK"
				diag := Diagnostic{}
				retryable := false
				if p.Err != nil {
					d := ClassifyCommandFailure(p.Err, p.Result)
					if p.Phase == "precheck" && expectedPrecheckMiss(p.Err, p.Result, step.Precheck) {
						state = "PRECHECK_MISS"
					} else {
						state = "PHASE_FAILED"
						diag = d
						retryable = installerStepRetryable(diag, step) && (step.MaxAttempts <= 0 || attempt < step.MaxAttempts)
					}
				}
				_ = e.Events.Event(ctx, Event{RunID: ir.ProvisionRunID, Step: name, Substep: p.Phase, State: state, Attempt: attempt, Diagnostic: diag, Duration: p.Duration, Retryable: retryable, Metadata: map[string]any{"installer": resolved.Manifest.Name, "installer_version": resolved.Manifest.Version}})
			}
		}, nil)
		if err != nil {
			diag := ClassifyCommandFailure(err, last)
			retryable := installerStepRetryable(diag, step)
			var re *ReadinessError
			if errors.As(err, &re) {
				retryable = re.Retryable
			}
			terminal := !retryable || (step.MaxAttempts > 0 && attempt >= step.MaxAttempts)
			if persistErr := e.Store.FinishStep(ctx, ir.ProvisionRunID, name, err, terminal); persistErr != nil {
				return errors.Join(err, persistErr)
			}
			state := "INSTALLING"
			if terminal {
				if len(resolved.Rollback) > 0 {
					state = "ROLLBACK_REQUIRED"
				} else {
					state = "FAILED"
				}
			}
			return errors.Join(err, e.States.SetState(ctx, ir.ID, state, err.Error()))
		}
		if err := e.Store.FinishStep(ctx, ir.ProvisionRunID, name, nil, false); err != nil {
			return err
		}
	}
	return e.States.SetState(ctx, ir.ID, "INSTALL_COMPLETE", "")
}
func (e InstallerExecutor) Rollback(ctx context.Context, ir InstallerRun, resolved ResolvedInstaller, target Target, key []byte) error {
	if len(resolved.Rollback) == 0 {
		return ErrInvalidPlan
	}
	for _, step := range resolved.Rollback {
		name := installerStepName(resolved.Manifest, ir.Generation, "rollback-"+step.Name)
		interrupted, err := e.Store.StepInterrupted(ctx, ir.ProvisionRunID, name)
		if err != nil {
			return err
		}
		if interrupted && step.Execute != "" && step.Precheck == "" {
			if err := e.Store.FinishStep(ctx, ir.ProvisionRunID, name, ErrInterruptedUnsafe, true); err != nil {
				return errors.Join(ErrInterruptedUnsafe, err)
			}
			return errors.Join(ErrInterruptedUnsafe, e.States.SetState(ctx, ir.ID, "FAILED", ErrInterruptedUnsafe.Error()))
		}
		if cs, ok := e.Store.(StepCompletionStore); ok {
			done, derr := cs.StepCompleted(ctx, ir.ProvisionRunID, name)
			if derr != nil {
				return derr
			}
			if done {
				continue
			}
		}
		attempt, err := e.Store.BeginStep(ctx, ir.ProvisionRunID, name, step.MaxAttempts)
		if err != nil {
			return err
		}
		var last CommandResult
		err = e.Scripts.RunScript(ctx, target, key, step, func(p ScriptPhaseResult) { last = p.Result }, nil)
		if err != nil {
			diag := ClassifyCommandFailure(err, last)
			retryable := installerStepRetryable(diag, step)
			terminal := !retryable || (step.MaxAttempts > 0 && attempt >= step.MaxAttempts)
			if persistErr := e.Store.FinishStep(ctx, ir.ProvisionRunID, name, err, terminal); persistErr != nil {
				return errors.Join(err, persistErr)
			}
			state := "ROLLBACK_REQUIRED"
			if terminal {
				state = "FAILED"
			}
			return errors.Join(err, e.States.SetState(ctx, ir.ID, state, "rollback: "+err.Error()))
		}
		if err := e.Store.FinishStep(ctx, ir.ProvisionRunID, name, nil, false); err != nil {
			return err
		}
		if e.Events != nil {
			_ = e.Events.Event(ctx, Event{RunID: ir.ProvisionRunID, Step: name, State: "ROLLBACK_OK", Attempt: attempt, Metadata: map[string]any{"installer": resolved.Manifest.Name}})
		}
	}
	return e.States.SetState(ctx, ir.ID, "ROLLED_BACK", "")
}
func installerStepName(m InstallerManifest, generation int, step string) string {
	return fmt.Sprintf("installer-g%d-%s-v%d-%s", generation, m.Name, m.Version, step)
}

// Unknown command outcomes are reconciled only through the immutable precheck.
// The caller still enforces the step's attempt budget and durable retry delay.
func installerStepRetryable(diag Diagnostic, step ScriptStep) bool {
	if diag.Code == "COMMAND_OUTCOME_UNKNOWN" {
		return step.Precheck != ""
	}
	return DiagnosticRetryable(diag)
}
