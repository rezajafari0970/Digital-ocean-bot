package provisioning

import (
	"context"
	"errors"
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
			run.State = Failed
			_ = e.Store.Update(ctx, run)
			return run, err
		}
	}
	run.State = Completed
	run.CurrentStep = "done"
	return run, e.Store.Update(ctx, run)
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
