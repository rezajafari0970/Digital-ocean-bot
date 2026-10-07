package app

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"testing"
	"time"
)

type remediationSteps struct{ calls map[string]int }

func (s *remediationSteps) call(d workflow.Deployment, name string) (workflow.Deployment, error) {
	s.calls[name]++
	return d, nil
}
func (s *remediationSteps) Create(_ context.Context, d workflow.Deployment) (workflow.Deployment, error) {
	return s.call(d, "create")
}
func (s *remediationSteps) WaitResource(_ context.Context, d workflow.Deployment) (workflow.Deployment, error) {
	return s.call(d, "wait_resource")
}
func (s *remediationSteps) Provision(_ context.Context, d workflow.Deployment) (workflow.Deployment, error) {
	return s.call(d, "provision")
}
func (s *remediationSteps) ImportDatabase(_ context.Context, d workflow.Deployment) (workflow.Deployment, error) {
	return s.call(d, "database")
}
func (s *remediationSteps) ConfigurePanel(_ context.Context, d workflow.Deployment) (workflow.Deployment, error) {
	return s.call(d, "panel")
}
func (s *remediationSteps) RegisterClients(_ context.Context, d workflow.Deployment, _ workflow.Request) (workflow.Deployment, error) {
	return s.call(d, "clients")
}
func (s *remediationSteps) RegisterTraffic(_ context.Context, d workflow.Deployment) (workflow.Deployment, error) {
	return s.call(d, "traffic")
}

type lostWorkflowReply struct {
	workflow.SQLStore
	lost bool
}

func (s *lostWorkflowReply) FinishStepResult(ctx context.Context, d workflow.Deployment, step string, stepErr error, class workflow.ErrorClass) error {
	if err := s.SQLStore.FinishStepResult(ctx, d, step, stepErr, class); err != nil {
		return err
	}
	if !s.lost {
		s.lost = true
		return errors.New("commit succeeded but acknowledgement lost")
	}
	return nil
}
func TestWorkflowDurableResultSurvivesLostReplyWithoutNativeReplay(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	ctx := context.Background()
	execBootstrap(t, db, "UPDATE deployments SET state='PLANNED',current_step='create' WHERE id=$1", f.d.ID)
	store := &lostWorkflowReply{SQLStore: workflow.SQLStore{DB: db}}
	steps := &remediationSteps{calls: map[string]int{}}
	e := workflow.Engine{Store: store, Steps: steps, Finalizer: deploymentReadyFinalizer{DB: db, Lifetime: time.Hour}}
	req := workflow.Request{DeploymentID: f.d.ID, AccountID: f.d.AccountID}
	if _, err := e.Run(ctx, req); err == nil {
		t.Fatal("lost reply not surfaced")
	}
	if steps.calls["create"] != 1 {
		t.Fatal(steps.calls)
	}
	d, err := e.Run(ctx, req)
	if err != nil || d.State != workflow.Ready {
		t.Fatal(d.State, err)
	}
	for name, n := range steps.calls {
		if n != 1 {
			t.Fatal("native replay", name, n)
		}
	}
	var attempts int
	if err = db.QueryRow("SELECT attempts FROM deployment_step_attempts WHERE deployment_id=$1 AND step='create'", d.ID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal(attempts, err)
	}
}
func TestWorkflowTerminalCommitIsAtomicAcrossEveryWrite(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	for _, terminal := range []string{"ready", "failed"} {
		for _, table := range []string{"deployments", "droplets", "provision_runs", "deployment_events"} {
			t.Run(terminal+"-"+table, func(t *testing.T) {
				f := newBootstrapFixture(t, db)
				step := "done"
				if terminal == "failed" {
					step = "create"
				}
				execBootstrap(t, db, "UPDATE deployments SET state='CREATING',current_step=$2 WHERE id=$1", f.d.ID, step)
				store := workflow.SQLStore{DB: db}
				if terminal == "failed" {
					if _, err := store.BeginStep(ctx, f.d.ID, "create", 3); err != nil {
						t.Fatal(err)
					}
					if err := store.FinishStep(ctx, f.d.ID, "create", workflow.ErrRuntimeConfig, workflow.ClassifyStepError("create", workflow.ErrRuntimeConfig)); err != nil {
						t.Fatal(err)
					}
				}
				event := "UPDATE"
				if table == "deployment_events" {
					event = "INSERT"
				}
				execBootstrap(t, db, "CREATE FUNCTION reject_engine_terminal() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'engine terminal injected'; END$$; CREATE TRIGGER reject_engine_terminal BEFORE "+event+" ON "+table+" FOR EACH ROW EXECUTE FUNCTION reject_engine_terminal()")
				steps := &remediationSteps{calls: map[string]int{}}
				e := workflow.Engine{Store: store, Steps: steps, Finalizer: deploymentReadyFinalizer{DB: db, Lifetime: time.Hour}, FailureFinalizer: deploymentFailureFinalizer{DB: db}}
				req := workflow.Request{DeploymentID: f.d.ID, AccountID: f.d.AccountID}
				if _, err := e.Run(ctx, req); err == nil {
					t.Fatal("terminal write failure hidden")
				}
				var ds, vs, ps string
				if err := db.QueryRow("SELECT d.state,v.state,p.state FROM deployments d JOIN droplets v ON v.id=d.droplet_id JOIN provision_runs p ON p.droplet_id=v.id WHERE d.id=$1", f.d.ID).Scan(&ds, &vs, &ps); err != nil {
					t.Fatal(err)
				}
				if ds != "CREATING" || vs != "PROVISIONING" || ps != "WAITING_INSTALLER" {
					t.Fatal("partial terminal state", ds, vs, ps)
				}
				execBootstrap(t, db, "DROP TRIGGER reject_engine_terminal ON "+table+"; DROP FUNCTION reject_engine_terminal()")
				d, err := e.Run(ctx, req)
				if terminal == "ready" {
					if err != nil || d.State != workflow.Ready {
						t.Fatal(d.State, err)
					}
				} else {
					if !errors.Is(err, workflow.ErrStepTerminal) || d.State != workflow.Failed {
						t.Fatal(d.State, err)
					}
				}
			})
		}
	}
}
func TestReadyFinalizerMissingProvisionRunRollsBackResource(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	execBootstrap(t, db, "DELETE FROM provision_runs WHERE id=$1", f.run)
	if err := (deploymentReadyFinalizer{DB: db, Lifetime: time.Hour}).MarkReady(context.Background(), f.d); err == nil {
		t.Fatal("missing run falsely READY")
	}
	var state string
	if err := db.QueryRow("SELECT state FROM droplets WHERE id=$1", f.d.DropletID).Scan(&state); err != nil || state != "PROVISIONING" {
		t.Fatal(state, err)
	}
}

func TestWorkflowRequiredEventFaultStopsNativeAndRecoversSavedResult(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	for _, faultStep := range []string{"create", "wait_resource"} {
		t.Run(faultStep, func(t *testing.T) {
			f := newBootstrapFixture(t, db)
			execBootstrap(t, db, "UPDATE deployments SET state='PLANNED',current_step='create' WHERE id=$1", f.d.ID)
			execBootstrap(t, db, "CREATE FUNCTION reject_step_event() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.step='"+faultStep+"' THEN RAISE EXCEPTION 'required event unavailable'; END IF; RETURN NEW; END$$; CREATE TRIGGER reject_step_event BEFORE INSERT ON deployment_events FOR EACH ROW EXECUTE FUNCTION reject_step_event()")
			steps := &remediationSteps{calls: map[string]int{}}
			store := workflow.SQLStore{DB: db}
			engine := workflow.Engine{Store: store, Steps: steps, Finalizer: deploymentReadyFinalizer{DB: db, Lifetime: time.Hour}}
			req := workflow.Request{DeploymentID: f.d.ID, AccountID: f.d.AccountID}
			if _, err := engine.Run(ctx, req); err == nil {
				t.Fatal("required event failure hidden")
			}
			if faultStep == "create" {
				for i := 0; i < 10; i++ {
					if _, err := engine.Run(ctx, req); err == nil {
						t.Fatal("event outage hidden")
					}
				}
				var attempts int
				if err := db.QueryRow("SELECT coalesce(sum(attempts),0) FROM deployment_step_attempts WHERE deployment_id=$1", f.d.ID).Scan(&attempts); err != nil || attempts != 0 {
					t.Fatal("unentered native budget consumed", attempts, err)
				}
			}
			if faultStep == "create" && steps.calls["create"] != 0 {
				t.Fatal("native executed before required event")
			}
			if faultStep == "wait_resource" && (steps.calls["create"] != 1 || steps.calls["wait_resource"] != 0) {
				t.Fatal("advanced after failed event", steps.calls)
			}
			execBootstrap(t, db, "DROP TRIGGER reject_step_event ON deployment_events; DROP FUNCTION reject_step_event()")
			d, err := engine.Run(ctx, req)
			if err != nil || d.State != workflow.Ready {
				t.Fatal(d.State, err)
			}
			for step, n := range steps.calls {
				if n != 1 {
					t.Fatal("repeated native step", step, n)
				}
			}
		})
	}
}
func TestMissingStepCompletionRowIsNeverAcknowledged(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	err := (workflow.SQLStore{DB: db}).FinishStepResult(context.Background(), f.d, "create", nil, "")
	if !errors.Is(err, workflow.ErrDeploymentVersionConflict) {
		t.Fatal(err)
	}
}
