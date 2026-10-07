package workflow

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"testing"
	"time"
)

type supervisedSteps struct {
	*stepStub
	provision func(context.Context, Deployment) (Deployment, error)
}

func (s supervisedSteps) Provision(c context.Context, d Deployment) (Deployment, error) {
	return s.provision(c, d)
}
func TestWorkflowUsesDeclaredPhaseBudget(t *testing.T) {
	now := time.Now()
	r := supervision.New(func() time.Time { return now })
	c, err := r.Register(context.Background(), "recovery", supervision.Policy{Loop: time.Second, Work: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	c, finish, err := supervision.Begin(c, "work", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	steps := supervisedSteps{stepStub: &stepStub{}, provision: func(ctx context.Context, d Deployment) (Deployment, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("native deadline lost")
		}
		now = now.Add(DefaultStepPolicies["provision"].Timeout / 2)
		if err := r.Check(); err != nil {
			t.Fatal("valid workflow phase killed", err)
		}
		if r.Snapshot().Modules[0].Active != 2 {
			t.Fatal("workflow stage untracked")
		}
		return d, nil
	}}
	d, err := (Engine{Store: &memStore{}, Steps: steps}).Run(c, Request{AccountID: "a", ProfileID: "p"})
	if err != nil || d.State != Ready {
		t.Fatal(d.State, err)
	}
	if err = r.Check(); err != nil {
		t.Fatal("parent deadline not resumed", err)
	}
}
