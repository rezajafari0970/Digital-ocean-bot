package app

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"testing"
)

func TestRecoveryTerminalBranchesPreserveStateOnPersistenceFailure(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	h := RecoveryHandler{Container: Container{DB: db}}
	for _, branch := range []string{"provision", "create"} {
		for _, fault := range []string{"state", "finalizer", "event"} {
			t.Run(branch+"-"+fault, func(t *testing.T) {
				f := newBootstrapFixture(t, db)
				state, step := "PROVISIONING", "provision"
				if branch == "create" {
					state, step = "CREATING", "create"
				}
				execBootstrap(t, db, "UPDATE deployments SET state=$2,current_step=$3,profile_snapshot=profile_snapshot||'{\"ssh_key_secret_ref\":\"fixture-secret\",\"ssh_provider_key_id\":\"fixture-key\"}'::jsonb WHERE id=$1", f.d.ID, state, step)
				if branch == "provision" {
					execBootstrap(t, db, "UPDATE provision_runs SET state='FAILED',last_error='terminal fixture' WHERE id=$1", f.run)
				} else {
					execBootstrap(t, db, "INSERT INTO operations(id,account_id,kind,state,idempotency_key) VALUES(gen_random_uuid(),$1,'CREATE_DROPLET','failed',$2)", f.d.AccountID, "deploy:"+f.d.ID+":create:fixture")
				}
				table, event := "deployments", "UPDATE"
				if fault == "finalizer" {
					table = "droplets"
				}
				if fault == "event" {
					table, event = "deployment_events", "INSERT"
				}
				execBootstrap(t, db, "CREATE FUNCTION reject_terminal() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'terminal persist injected'; END$$; CREATE TRIGGER reject_terminal BEFORE "+event+" ON "+table+" FOR EACH ROW EXECUTE FUNCTION reject_terminal()")
				item := worker.RecoveryItem{ID: f.d.ID, AccountID: f.d.AccountID}
				if err := h.RecoverDeployment(ctx, item); err == nil {
					t.Fatal("handler acknowledged failed terminal persistence")
				}
				var ds, vs string
				if err := db.QueryRow("SELECT d.state,v.state FROM deployments d JOIN droplets v ON v.id=d.droplet_id WHERE d.id=$1", f.d.ID).Scan(&ds, &vs); err != nil {
					t.Fatal(err)
				}
				if ds != state || vs != "PROVISIONING" {
					t.Fatal("partial terminal transaction", ds, vs)
				}
				w := worker.Worker{Store: worker.RecoveryStore{DB: db}, Failures: worker.FailureStore{DB: db}, Handler: h}
				if err := w.Once(ctx); err != nil {
					t.Fatal(err)
				}
				var n int
				if err := db.QueryRow("SELECT failures FROM worker_item_failures WHERE kind='deployment' AND item_id=$1", f.d.ID).Scan(&n); err != nil || n < 1 {
					t.Fatal("terminal failure evidence not durable", n, err)
				}
				execBootstrap(t, db, "DROP TRIGGER reject_terminal ON "+table+"; DROP FUNCTION reject_terminal(); UPDATE worker_item_failures SET next_retry_at=now()-interval '1 second'")
				if err := w.Once(ctx); err != nil {
					t.Fatal(err)
				}
				d, err := (workflow.SQLStore{DB: db}).Get(ctx, f.d.ID, f.d.AccountID)
				if err != nil || d.State != workflow.Failed {
					t.Fatal(d.State, err)
				}
				if err = db.QueryRow("SELECT state FROM droplets WHERE id=$1", f.d.DropletID).Scan(&vs); err != nil || vs != "RETIRING" {
					t.Fatal(vs, err)
				}
			})
		}
	}
}
func TestInstallerTerminalStateAndFinalizerAreAtomic(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c := Container{DB: db}
	f := newBootstrapFixture(t, db)
	execBootstrap(t, db, "CREATE FUNCTION reject_installer_final() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'finalizer unavailable'; END$$; CREATE TRIGGER reject_installer_final BEFORE UPDATE ON droplets FOR EACH ROW EXECUTE FUNCTION reject_installer_final()")
	if err := c.setInstallerDeploymentState(ctx, f.d, workflow.InstallFailed, "installer_failed", "fixture"); err == nil {
		t.Fatal("failure swallowed")
	}
	d, err := (workflow.SQLStore{DB: db}).Get(ctx, f.d.ID, f.d.AccountID)
	if err != nil || d.State != workflow.WaitingInstaller {
		t.Fatal("partial state committed", d.State, err)
	}
	execBootstrap(t, db, "DROP TRIGGER reject_installer_final ON droplets")
	if err = c.setInstallerDeploymentState(ctx, f.d, workflow.InstallFailed, "installer_failed", "fixture"); err != nil {
		t.Fatal(err)
	}
}

func TestOperationRecoveryRejectsUncommittedConditionalUpdate(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	ctx := context.Background()
	var id string
	if err := db.QueryRow("INSERT INTO operations(id,account_id,kind,state,idempotency_key) VALUES(gen_random_uuid(),$1,'DELETE_DROPLET','unknown','conditional-recovery-fixture') RETURNING id", f.d.AccountID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	item := worker.RecoveryItem{ID: id, AccountID: f.d.AccountID, Kind: "DELETE_DROPLET"}
	h := RecoveryHandler{Container: Container{DB: db}}
	// PostgreSQL reports UPDATE 0 without returning a SQL error.
	execBootstrap(t, db, "CREATE FUNCTION skip_recovery_update() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.state='failed' THEN RETURN NULL; END IF; RETURN NEW; END$$; CREATE TRIGGER skip_recovery_update BEFORE UPDATE ON operations FOR EACH ROW EXECUTE FUNCTION skip_recovery_update()")
	if err := h.RecoverOperation(ctx, item); err == nil {
		t.Fatal("uncommitted terminal outcome acknowledged")
	}
	var state string
	if err := db.QueryRow("SELECT state FROM operations WHERE id=$1", id).Scan(&state); err != nil || state != "unknown" {
		t.Fatal(state, err)
	}
	execBootstrap(t, db, "DROP TRIGGER skip_recovery_update ON operations; DROP FUNCTION skip_recovery_update()")
	if err := h.RecoverOperation(ctx, item); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT state FROM operations WHERE id=$1", id).Scan(&state); err != nil || state != "failed" {
		t.Fatal(state, err)
	}
	// A state/version already changed by another owner is never falsely acked.
	if err := h.RecoverOperation(ctx, item); err == nil {
		t.Fatal("zero-row state conflict acknowledged")
	}
}

type recoveryRowsResult struct {
	n   int64
	err error
}

func (r recoveryRowsResult) LastInsertId() (int64, error) { return 0, nil }
func (r recoveryRowsResult) RowsAffected() (int64, error) { return r.n, r.err }

func TestRecoveryOperationRequiresDurableTransition(t *testing.T) {
	fault := errors.New("rows unavailable")
	for _, n := range []int64{0, 2, -1} {
		if requireRecoveryOperationUpdate(recoveryRowsResult{n: n}, nil) == nil {
			t.Fatalf("accepted affected rows=%d", n)
		}
	}
	if err := requireRecoveryOperationUpdate(recoveryRowsResult{n: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(requireRecoveryOperationUpdate(recoveryRowsResult{err: fault}, nil), fault) {
		t.Fatal("row error lost")
	}
	if !errors.Is(requireRecoveryOperationUpdate(nil, fault), fault) {
		t.Fatal("write error lost")
	}
	var _ sql.Result = recoveryRowsResult{}
}

func TestRecoveryOperationSuppressedWriteDoesNotAcknowledge(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	var operation string
	if err := db.QueryRow("INSERT INTO operations(id,account_id,kind,state,idempotency_key) VALUES(gen_random_uuid(),$1,'DELETE_DROPLET','unknown','zero-write-recovery') RETURNING id", f.d.AccountID).Scan(&operation); err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, "CREATE FUNCTION suppress_recovery_update() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RETURN NULL; END$$; CREATE TRIGGER suppress_recovery_update BEFORE UPDATE ON operations FOR EACH ROW EXECUTE FUNCTION suppress_recovery_update()")
	h := RecoveryHandler{Container: Container{DB: db}}
	item := worker.RecoveryItem{ID: operation, AccountID: f.d.AccountID, Kind: "DELETE_DROPLET"}
	if err := h.RecoverOperation(context.Background(), item); err == nil {
		t.Fatal("zero-row update falsely acknowledged")
	}
	var state string
	if err := db.QueryRow("SELECT state FROM operations WHERE id=$1", operation).Scan(&state); err != nil || state != "unknown" {
		t.Fatal(state, err)
	}
	execBootstrap(t, db, "DROP TRIGGER suppress_recovery_update ON operations")
	if err := h.RecoverOperation(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT state FROM operations WHERE id=$1", operation).Scan(&state); err != nil || state != "failed" {
		t.Fatal(state, err)
	}
}

func TestFailureFinalizerRejectsSuppressedAndIncompatibleState(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	for _, fault := range []string{"droplet-zero", "run-zero", "run-missing", "run-completed", "resource-ready", "resource-missing"} {
		t.Run(fault, func(t *testing.T) {
			f := newBootstrapFixture(t, db)
			switch fault {
			case "droplet-zero", "run-zero":
				table := "droplets"
				if fault == "run-zero" {
					table = "provision_runs"
				}
				execBootstrap(t, db, "CREATE FUNCTION suppress_terminal() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RETURN NULL; END$$; CREATE TRIGGER suppress_terminal BEFORE UPDATE ON "+table+" FOR EACH ROW EXECUTE FUNCTION suppress_terminal()")
				defer execBootstrap(t, db, "DROP TRIGGER suppress_terminal ON "+table+"; DROP FUNCTION suppress_terminal()")
			case "run-missing":
				execBootstrap(t, db, "DELETE FROM provision_runs WHERE id=$1", f.run)
			case "run-completed":
				execBootstrap(t, db, "UPDATE provision_runs SET state='COMPLETED' WHERE id=$1", f.run)
			case "resource-ready":
				execBootstrap(t, db, "UPDATE droplets SET state='READY' WHERE id=$1", f.d.DropletID)
			case "resource-missing":
				f.d.DropletID = "00000000-0000-4000-8000-000000000001"
			}
			h := RecoveryHandler{Container: Container{DB: db}}
			if err := h.persistTerminalRecovery(ctx, f.d, "fixture", "FIXTURE"); err == nil {
				t.Fatal("incomplete finalization acknowledged", fault)
			}
			d, err := (workflow.SQLStore{DB: db}).Get(ctx, f.d.ID, f.d.AccountID)
			if err != nil || d.State != workflow.WaitingInstaller || d.LockVersion != f.d.LockVersion {
				t.Fatal("deployment committed without terminal dependencies", d, err)
			}
			var events int
			if err = db.QueryRow("SELECT count(*) FROM deployment_events WHERE deployment_id=$1", f.d.ID).Scan(&events); err != nil || events != 0 {
				t.Fatal("partial terminal event", events, err)
			}
			var resource string
			if err = db.QueryRow("SELECT state FROM droplets WHERE id=$1", d.DropletID).Scan(&resource); err != nil {
				t.Fatal(err)
			}
			expected := "PROVISIONING"
			if fault == "resource-ready" {
				expected = "READY"
			}
			if resource != expected {
				t.Fatal("partial resource change", resource, expected)
			}
		})
	}
}
func TestFailureFinalizerIdempotentRetirementAndRun(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	for _, state := range []string{"RETIRING", "DELETING", "DELETED"} {
		t.Run(state, func(t *testing.T) {
			f := newBootstrapFixture(t, db)
			execBootstrap(t, db, "UPDATE droplets SET state=$2 WHERE id=$1", f.d.DropletID, state)
			execBootstrap(t, db, "UPDATE provision_runs SET state='FAILED',next_retry_at=now() WHERE id=$1", f.run)
			finalizer := deploymentFailureFinalizer{DB: db}
			for i := 0; i < 2; i++ {
				if err := finalizer.MarkFailed(ctx, f.d); err != nil {
					t.Fatal(err)
				}
			}
			var rs, ps string
			var retry sql.NullTime
			var events int
			if err := db.QueryRow("SELECT d.state,p.state,p.next_retry_at FROM droplets d JOIN provision_runs p ON p.droplet_id=d.id WHERE d.id=$1", f.d.DropletID).Scan(&rs, &ps, &retry); err != nil {
				t.Fatal(err)
			}
			if rs != state || ps != "FAILED" || retry.Valid {
				t.Fatal(rs, ps, retry)
			}
			if err := db.QueryRow("SELECT count(*) FROM lifecycle_events WHERE resource_id=$1", f.d.DropletID).Scan(&events); err != nil || events != 0 {
				t.Fatal("duplicate retirement event", events, err)
			}
		})
	}
}
