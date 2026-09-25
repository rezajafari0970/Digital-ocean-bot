package provisioning

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidPlan = errors.New("invalid provision plan")

type SecretReader interface {
	Get(context.Context, string, string) ([]byte, error)
}
type CommandRunner interface {
	Wait(context.Context, Target, []byte) error
	Run(context.Context, Target, []byte, string) (string, error)
}
type Plan struct {
	Bootstrap    string
	InstallPanel string
	Verify       string
}
type Engine struct {
	Store   Store
	Secrets SecretReader
	SSH     CommandRunner
}

func (e Engine) Execute(ctx context.Context, target Target, plan Plan) (Run, error) {
	if plan.Bootstrap == "" || plan.InstallPanel == "" || plan.Verify == "" {
		return Run{}, ErrInvalidPlan
	}
	run, fresh, err := e.Store.Reserve(ctx, Run{AccountID: target.AccountID, DropletID: target.DropletID, State: Pending, CurrentStep: "ssh"})
	if err != nil {
		return Run{}, err
	}
	if !fresh && run.State == Completed {
		return run, nil
	}
	if !fresh && run.NextRetryAt != nil && time.Now().Before(*run.NextRetryAt) {
		return run, fmt.Errorf("provision retry deferred until %s", run.NextRetryAt.UTC().Format(time.RFC3339))
	}
	const maxAttempts = 8
	if !fresh && run.Attempt >= maxAttempts {
		run.State = Failed
		if run.LastError == "" {
			run.LastError = "provision retry limit reached"
		}
		_ = e.Store.Update(ctx, run)
		return run, fmt.Errorf("provision retry limit reached: %s", run.LastError)
	}
	key, err := e.Secrets.Get(ctx, target.AccountID, target.KeySecretRef)
	if err != nil {
		return run, err
	}
	defer wipe(key)
	steps := []struct {
		name    string
		state   State
		command string
	}{{"ssh", WaitingSSH, ""}, {"bootstrap", Bootstrapping, plan.Bootstrap}, {"panel", InstallingPanel, plan.InstallPanel}, {"verify", Verifying, plan.Verify}}
	for _, step := range steps {
		if completed(run.CurrentStep, step.name) {
			continue
		}
		run.State = step.state
		run.CurrentStep = step.name
		run.Attempt++
		if err := e.Store.Update(ctx, run); err != nil {
			return run, err
		}
		if step.name == "ssh" {
			err = e.SSH.Wait(ctx, target, key)
		} else {
			_, err = e.SSH.Run(ctx, target, key, step.command)
		}
		if err != nil {
			run.LastError = err.Error()
			backoff := time.Duration(1<<min(run.Attempt, 6)) * time.Minute
			next := time.Now().Add(backoff)
			run.NextRetryAt = &next
			// Context cancellation/timeout is an interrupted attempt, not a
			// permanent provisioning failure. Persist the current resumable step.
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				run.State = Failed
			}
			_ = e.Store.Update(ctx, run)
			return run, err
		}
		// Persist the checkpoint immediately. A killed worker can then resume
		// at the next step instead of repeating an already successful command.
		run.CurrentStep = provisionNext(step.name)
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

func provisionNext(s string) string {
	m := map[string]string{
		"ssh":       "bootstrap",
		"bootstrap": "panel",
		"panel":     "verify",
		"verify":    "done",
	}
	return m[s]
}

func completed(current, target string) bool {
	order := map[string]int{"ssh": 0, "bootstrap": 1, "panel": 2, "verify": 3, "done": 4}
	return order[current] > order[target]
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
