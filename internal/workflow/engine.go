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
type Engine struct {
	Store Store
	Steps Steps
}

func (e Engine) Run(ctx context.Context, req Request) (Deployment, error) {
	d, _, err := e.Store.Reserve(ctx, req)
	if err != nil {
		return d, err
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
		d.State = step.state
		d.CurrentStep = step.name
		d.Attempt++
		if err := e.Store.Update(ctx, d); err != nil {
			return d, err
		}
		_ = e.Store.Event(ctx, d.ID, step.name, d.State, "")
		d, err = step.fn(ctx, d)
		if err != nil {
			d.LastError = err.Error()
			_ = e.Store.Update(ctx, d)
			_ = e.Store.Event(ctx, d.ID, step.name, d.State, "step failed")
			return d, err
		}
		d.CurrentStep = next(step.name)
		if err := e.Store.Update(ctx, d); err != nil {
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
