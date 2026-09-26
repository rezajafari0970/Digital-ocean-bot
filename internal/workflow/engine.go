package workflow

import (
	"context"
	"errors"
)

var ErrStepUnavailable = errors.New("workflow step unavailable")

type Steps interface {
	Create(context.Context, Deployment) (Deployment, error)
	WaitResource(context.Context, Deployment) (Deployment, error)
	Provision(context.Context, Deployment) (Deployment, error)
	ImportDatabase(context.Context, Deployment) (Deployment, error)
	ConfigurePanel(context.Context, Deployment) (Deployment, error)
	RegisterClients(context.Context, Deployment, Request) (Deployment, error)
	RegisterTraffic(context.Context, Deployment) (Deployment, error)
}
type ReadyFinalizer interface {
	MarkReady(context.Context, Deployment) error
}
type FailureFinalizer interface {
	MarkFailed(context.Context, Deployment) error
}

type Engine struct {
	Store            Store
	Steps            Steps
	Finalizer        ReadyFinalizer
	FailureFinalizer FailureFinalizer
	RunLock          interface {
		Lock()
		Unlock()
	}
	RunLease RunLease
}

func (e Engine) Run(ctx context.Context, req Request) (Deployment, error) {
	if e.RunLock != nil {
		e.RunLock.Lock()
		defer e.RunLock.Unlock()
	}
	d, _, err := e.Store.Reserve(ctx, req)
	if err != nil {
		return d, err
	}
	if e.RunLease != nil {
		release, lockErr := e.RunLease.Acquire(ctx, d.ID)
		if lockErr != nil {
			return d, lockErr
		}
		defer release()
		// Another runner may have completed or advanced the deployment before
		// this lease was acquired. Re-read persisted state under the lease.
		d, _, err = e.Store.Reserve(ctx, Request{DeploymentID: d.ID, AccountID: d.AccountID, ProfileID: d.ProfileID})
		if err != nil {
			return d, err
		}
	}
	if d.State == Ready {
		return d, nil
	}
	steps := []struct {
		name  string
		state State
		fn    func(context.Context, Deployment) (Deployment, error)
	}{{"create", Creating, e.Steps.Create}, {"wait_resource", WaitingResource, e.Steps.WaitResource}, {"provision", Provisioning, e.Steps.Provision}, {"database", ImportingDatabase, e.Steps.ImportDatabase}, {"panel", ConfiguringPanel, e.Steps.ConfigurePanel}, {"clients", RegisteringClients, func(c context.Context, x Deployment) (Deployment, error) { return e.Steps.RegisterClients(c, x, req) }}, {"traffic", RegisteringTraffic, e.Steps.RegisterTraffic}}
	for _, step := range steps {
		if done(d.CurrentStep, step.name) {
			continue
		}
		policy := DefaultStepPolicies[step.name]
		_, beginErr := e.Store.BeginStep(ctx, d.ID, step.name, policy.MaxAttempts)
		if beginErr != nil {
			if errors.Is(beginErr, ErrStepRetryDeferred) {
				return d, beginErr
			}
			d.State = Failed
			d.CurrentStep = "done"
			d.LastError = beginErr.Error()
			_ = e.Store.Update(ctx, d)
			if e.FailureFinalizer != nil {
				_ = e.FailureFinalizer.MarkFailed(ctx, d)
			}
			code := "STEP_TERMINAL_FAILURE"
			if errors.Is(beginErr, ErrStepRetryLimit) {
				code = "STEP_RETRY_LIMIT"
			}
			_ = e.Store.Event(ctx, d.ID, step.name, Failed, code)
			return d, beginErr
		}
		d.State = step.state
		d.CurrentStep = step.name
		d.Attempt++
		if err := e.Store.Update(ctx, d); err != nil {
			return d, err
		}
		_ = e.Store.Event(ctx, d.ID, step.name, d.State, "")
		stepCtx := ctx
		cancel := func() {}
		if policy.Timeout > 0 {
			stepCtx, cancel = context.WithTimeout(ctx, policy.Timeout)
		}
		d, err = step.fn(stepCtx, d)
		cancel()
		class := ClassifyStepError(step.name, err)
		_ = e.Store.FinishStep(ctx, d.ID, step.name, err, class)
		if err != nil {
			d.LastError = err.Error()
			if !RetryableClass(class) {
				d.State = Failed
				d.CurrentStep = "done"
			}
			_ = e.Store.Update(ctx, d)
			if d.State == Failed && e.FailureFinalizer != nil {
				_ = e.FailureFinalizer.MarkFailed(ctx, d)
			}
			_ = e.Store.Event(ctx, d.ID, step.name, d.State, "step failed class="+string(class))
			return d, err
		}
		d.CurrentStep = next(step.name)
		if err := e.Store.Update(ctx, d); err != nil {
			return d, err
		}
	}

	if e.Finalizer != nil {
		if err := e.Finalizer.MarkReady(ctx, d); err != nil {
			return d, err
		}
	}
	d.State = Ready
	d.CurrentStep = "done"
	d.LastError = ""
	if err := e.Store.Update(ctx, d); err != nil {
		return d, err
	}
	_ = e.Store.Event(ctx, d.ID, "done", Ready, "")
	return d, nil
}

func done(current, target string) bool { return rank(current) > rank(target) }
func rank(s string) int {
	m := map[string]int{"create": 0, "wait_resource": 1, "provision": 2, "database": 3, "panel": 4, "clients": 5, "traffic": 6, "done": 7}
	return m[s]
}
func next(s string) string {
	m := map[string]string{"create": "wait_resource", "wait_resource": "provision", "provision": "database", "database": "panel", "panel": "clients", "clients": "traffic", "traffic": "done"}
	return m[s]
}
