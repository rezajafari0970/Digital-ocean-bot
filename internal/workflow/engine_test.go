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
func (s *memStore) Update(_ context.Context, d *Deployment) error {
	s.d = *d
	d.LockVersion++
	return nil
}
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

func TestWorkflowTerminalStatesDoNotResume(t *testing.T) {
	for _, state := range []State{Ready, Failed, InstallFailed, InstallRolledBack, PanelComplete} {
		t.Run(string(state), func(t *testing.T) {
			store := &memStore{d: Deployment{ID: "1", AccountID: "a", ProfileID: "p", State: state, CurrentStep: "done"}, exists: true}
			steps := &stepStub{}
			d, err := (Engine{Store: store, Steps: steps, PostInstallOnly: true}).Run(context.Background(), Request{DeploymentID: "1", AccountID: "a", ProfileID: "p"})
			if err != nil {
				t.Fatal(err)
			}
			if d.State != state {
				t.Fatalf("state changed: got %s want %s", d.State, state)
			}
			if steps.calls != 0 {
				t.Fatalf("terminal deployment executed %d steps", steps.calls)
			}
		})
	}
}

func TestInstallCompleteContinuesThroughDatabaseAndPanelThenStopsSafely(t *testing.T) {
	store := &memStore{d: Deployment{ID: "1", AccountID: "a", ProfileID: "p", State: InstallComplete, CurrentStep: "installer_complete"}, exists: true}
	steps := &stepStub{}
	d, err := (Engine{Store: store, Steps: steps, PostInstallOnly: true}).Run(context.Background(), Request{DeploymentID: "1", AccountID: "a", ProfileID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if d.State != PanelComplete || d.CurrentStep != "panel_complete" {
		t.Fatalf("got state=%s step=%s", d.State, d.CurrentStep)
	}
	if steps.calls != 2 {
		t.Fatalf("post-installer continuation executed %d steps, want database+panel", steps.calls)
	}
}

func TestDatabaseCompleteResumesAtPanelOnly(t *testing.T) {
	store := &memStore{d: Deployment{ID: "1", AccountID: "a", ProfileID: "p", State: DatabaseComplete, CurrentStep: "database_complete"}, exists: true}
	steps := &stepStub{}
	d, err := (Engine{Store: store, Steps: steps, PostInstallOnly: true}).Run(context.Background(), Request{DeploymentID: "1", AccountID: "a", ProfileID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if d.State != PanelComplete || steps.calls != 1 {
		t.Fatalf("got state=%s calls=%d", d.State, steps.calls)
	}
}

func (s *memStore) Finalize(ctx context.Context, d *Deployment, step, message string, apply func(context.Context, DBTX) error) error {
	if apply != nil {
		if err := apply(ctx, nil); err != nil {
			return err
		}
	}
	return s.Update(ctx, d)
}

func (s *memStore) AdmitStep(ctx context.Context, d *Deployment, step string, max int) error {
	if _, err := s.BeginStep(ctx, d.ID, step, max); err != nil {
		return err
	}
	return s.Update(ctx, d)
}
