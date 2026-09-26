package provisioning

import (
	"context"
	"errors"
	"time"
)

var ErrScriptVerify = errors.New("script verification failed")

type ScriptPhaseResult struct {
	Phase    string
	Result   CommandResult
	Err      error
	Duration time.Duration
	Skipped  bool
}

type ScriptExecutor interface {
	RunScript(context.Context, Target, []byte, ScriptStep, func(ScriptPhaseResult), StageObserver) error
}

type SSHScriptRunner struct{ SSH StagedCommandRunner }

func (r SSHScriptRunner) RunScript(ctx context.Context, t Target, key []byte, step ScriptStep, observe func(ScriptPhaseResult), stages StageObserver) error {
	timeout := step.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	run := func(phase, cmd string) (CommandResult, error) {
		phaseCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		start := time.Now()
		res, err := r.SSH.RunDetailedObserved(phaseCtx, t, key, cmd, stages)
		if observe != nil {
			observe(ScriptPhaseResult{Phase: phase, Result: res, Err: err, Duration: time.Since(start)})
		}
		return res, err
	}
	if step.Precheck != "" {
		precheckResult, err := run("precheck", step.Precheck)
		if err == nil {
			if observe != nil {
				observe(ScriptPhaseResult{Phase: "execute", Skipped: true})
			}
			if step.Verify == "" {
				return nil
			}
			_, err = run("verify", step.Verify)
			if err != nil {
				return errors.Join(ErrScriptVerify, err)
			}
			return nil
		}
		if !errors.Is(err, ErrSSHCommand) {
			return err
		}
		diag := ClassifyCommandFailure(err, precheckResult)
		// A plain command exit means the idempotency predicate is unmet.
		// Recognized infrastructure/system failures must not be mistaken for
		// "not installed" and are propagated to retry/terminal policy.
		if diag.Code != "SSH_COMMAND_FAILED" && diag.Code != "SSH_COMMAND_EXIT" {
			return err
		}
	}
	if step.Execute != "" {
		if _, err := run("execute", step.Execute); err != nil {
			return err
		}
	}
	if step.Verify != "" {
		if _, err := run("verify", step.Verify); err != nil {
			return errors.Join(ErrScriptVerify, err)
		}
	}
	return nil
}
