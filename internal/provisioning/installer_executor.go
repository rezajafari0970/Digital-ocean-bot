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
	for _, step := range resolved.Steps {
		name := installerStepName(resolved.Manifest, step.Name)
		attempt, err := e.Store.BeginStep(ctx, ir.ProvisionRunID, name, step.MaxAttempts)
		if err != nil {
			_ = e.States.SetState(ctx, ir.ID, "FAILED", err.Error())
			return err
		}
		var last CommandResult
		err = e.Scripts.RunScript(ctx, target, key, step, func(p ScriptPhaseResult) {
			last = p.Result
			if e.Events != nil {
				state := "PHASE_OK"
				diag := Diagnostic{}
				retryable := false
				if p.Err != nil {
					state = "PHASE_FAILED"
					diag = ClassifyCommandFailure(p.Err, p.Result)
					retryable = DiagnosticRetryable(diag)
				}
				_ = e.Events.Event(ctx, Event{RunID: ir.ProvisionRunID, Step: name, Substep: p.Phase, State: state, Attempt: attempt, Diagnostic: diag, Duration: p.Duration, Retryable: retryable, Metadata: map[string]any{"installer": resolved.Manifest.Name, "installer_version": resolved.Manifest.Version}})
			}
		}, nil)
		if err != nil {
			diag := ClassifyCommandFailure(err, last)
			retryable := DiagnosticRetryable(diag)
			var re *ReadinessError
			if errors.As(err, &re) {
				retryable = re.Retryable
			}
			if diag.Code == "COMMAND_OUTCOME_UNKNOWN" && step.Precheck == "" {
				retryable = false
			}
			terminal := !retryable
			_ = e.Store.FinishStep(ctx, ir.ProvisionRunID, name, err, terminal)
			state := "INSTALLING"
			if terminal {
				if len(resolved.Rollback) > 0 {
					state = "ROLLBACK_REQUIRED"
				} else {
					state = "FAILED"
				}
			}
			_ = e.States.SetState(ctx, ir.ID, state, err.Error())
			return err
		}
		_ = e.Store.FinishStep(ctx, ir.ProvisionRunID, name, nil, false)
	}
	return e.States.SetState(ctx, ir.ID, "INSTALL_COMPLETE", "")
}
func (e InstallerExecutor) Rollback(ctx context.Context, ir InstallerRun, resolved ResolvedInstaller, target Target, key []byte) error {
	if len(resolved.Rollback) == 0 {
		return ErrInvalidPlan
	}
	for _, step := range resolved.Rollback {
		name := installerStepName(resolved.Manifest, "rollback-"+step.Name)
		attempt, err := e.Store.BeginStep(ctx, ir.ProvisionRunID, name, step.MaxAttempts)
		if err != nil {
			return err
		}
		err = e.Scripts.RunScript(ctx, target, key, step, nil, nil)
		_ = e.Store.FinishStep(ctx, ir.ProvisionRunID, name, err, err != nil)
		if err != nil {
			_ = e.States.SetState(ctx, ir.ID, "FAILED", "rollback: "+err.Error())
			return err
		}
		if e.Events != nil {
			_ = e.Events.Event(ctx, Event{RunID: ir.ProvisionRunID, Step: name, State: "ROLLBACK_OK", Attempt: attempt, Metadata: map[string]any{"installer": resolved.Manifest.Name}})
		}
	}
	return e.States.SetState(ctx, ir.ID, "ROLLED_BACK", "")
}
func installerStepName(m InstallerManifest, step string) string {
	return fmt.Sprintf("installer-%s-v%d-%s", m.Name, m.Version, step)
}
