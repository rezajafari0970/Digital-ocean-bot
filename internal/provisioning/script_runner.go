package provisioning

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
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
		stageCtx, finish, err := supervision.Begin(ctx, phase, timeout+10*time.Second)
		if err != nil {
			return CommandResult{}, err
		}
		defer finish()
		phaseCtx, cancel := context.WithTimeout(stageCtx, timeout)
		defer cancel()
		start := time.Now()
		// A failed idempotency predicate is an expected precheck miss. Buffer its
		// command observation until stderr/result classification is available so it
		// is not also recorded as an SSH failure. Transport and real command faults
		// still reach the stage observer unchanged.
		var commandStages []StageObservation
		phaseStages := stages
		if phase == "precheck" && stages != nil {
			phaseStages = func(o StageObservation) {
				if o.Stage == StageCommand {
					commandStages = append(commandStages, o)
				} else {
					stages(o)
				}
			}
		}
		res, err := r.SSH.RunDetailedObserved(phaseCtx, t, key, cmd, phaseStages)
		expectedMiss := phase == "precheck" && expectedPrecheckMiss(err, res)
		for _, o := range commandStages {
			if !expectedMiss || o.Err == nil {
				stages(o)
			}
		}
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
		if !expectedPrecheckMiss(err, precheckResult) {
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

// Only the documented false-predicate exit may authorize execution. Shell
// errors, signals, missing exit status and infrastructure faults are failures.
func expectedPrecheckMiss(err error, result CommandResult) bool {
	if !errors.Is(err, ErrSSHCommand) || result.ExitCode == nil || *result.ExitCode != 1 || result.Signal != "" {
		return false
	}
	diag := ClassifyCommandFailure(err, result)
	return diag.Code == "SSH_COMMAND_EXIT" || diag.Code == "SSH_COMMAND_FAILED"
}
