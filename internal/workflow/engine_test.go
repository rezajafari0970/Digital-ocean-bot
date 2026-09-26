package workflow

import (
	"context"
	"testing"
)

type memStore struct {
	d      Deployment
	exists bool
}

func (s *memStore) Reserve(_ context.Context, r Request) (Deployment, bool, error) {
	if s.exists {
		return s.d, false, nil
	}
	s.d = Deployment{ID: "1", AccountID: r.AccountID, ProfileID: r.ProfileID, State: Planned, CurrentStep: "create"}
	s.exists = true
	return s.d, true, nil
}
func (s *memStore) Update(_ context.Context, d Deployment) error                        { s.d = d; return nil }
func (s *memStore) Event(context.Context, string, string, State, string) error          { return nil }
func (s *memStore) BeginStep(context.Context, string, string, int) (int, error)         { return 1, nil }
func (s *memStore) FinishStep(context.Context, string, string, error, ErrorClass) error { return nil }

type stepStub struct{ calls int }

func (s *stepStub) x(d Deployment) (Deployment, error)                                 { s.calls++; return d, nil }
func (s *stepStub) Create(_ context.Context, d Deployment) (Deployment, error)         { return s.x(d) }
func (s *stepStub) WaitResource(_ context.Context, d Deployment) (Deployment, error)   { return s.x(d) }
func (s *stepStub) Provision(_ context.Context, d Deployment) (Deployment, error)      { return s.x(d) }
func (s *stepStub) ImportDatabase(_ context.Context, d Deployment) (Deployment, error) { return s.x(d) }
func (s *stepStub) ConfigurePanel(_ context.Context, d Deployment) (Deployment, error) { return s.x(d) }
func (s *stepStub) RegisterClients(_ context.Context, d Deployment, _ Request) (Deployment, error) {
	return s.x(d)
}
func (s *stepStub) RegisterTraffic(_ context.Context, d Deployment) (Deployment, error) {
	return s.x(d)
}

func TestWorkflowRunsOnceAndResumesCompleted(t *testing.T) {
	store := &memStore{}
	steps := &stepStub{}
	e := Engine{Store: store, Steps: steps}
	req := Request{AccountID: "a", ProfileID: "p"}
	d, err := e.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if d.State != Ready || steps.calls != 7 {
		t.Fatalf("state=%s calls=%d", d.State, steps.calls)
	}
	_, err = e.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if steps.calls != 7 {
		t.Fatal("ready deployment repeated")
	}
}
