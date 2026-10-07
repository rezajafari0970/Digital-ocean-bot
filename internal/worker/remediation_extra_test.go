package worker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProcessRoleFenceSurvivesSQLBackendLossAndMixedMode(t *testing.T) {
	db := failureLedgerFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	owner, err := AcquireProcessRole(root, RoleControl)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	panels, err := AcquireProcessRole(root, RolePanels)
	if err != nil {
		t.Fatal(err)
	}
	defer panels.Close()
	lease, err := AcquireRole(ctx, db, RoleControl)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if _, err = db.ExecContext(ctx, "SELECT pg_terminate_backend($1)", lease.backend); err != nil {
		t.Fatal(err)
	}
	if err = lease.Check(ctx); err == nil {
		t.Fatal("dead database owner accepted")
	}
	// Even though the SQL lock is gone, neither replacement mode can start.
	for _, role := range []Role{RoleControl, RoleAll} {
		replacement, err := AcquireProcessRole(root, role)
		if replacement != nil {
			replacement.Close()
		}
		if !errors.Is(err, ErrRoleOwned) {
			t.Fatal(role, err)
		}
	}
	owner.Close()
	if replacement, err := AcquireProcessRole(root, RoleAll); !errors.Is(err, ErrRoleOwned) {
		if replacement != nil {
			replacement.Close()
		}
		t.Fatal(err)
	}
	control, err := AcquireProcessRole(root, RoleControl)
	if err != nil {
		t.Fatal("failed all acquisition leaked control lock", err)
	}
	control.Close()
	panels.Close()
	all, err := AcquireProcessRole(root, RoleAll)
	if err != nil {
		t.Fatal(err)
	}
	defer all.Close()
}

type selectiveBypassProbe struct {
	recoveryProbe
	target string
}

func (p *selectiveBypassProbe) BypassDeploymentBackoff(_ context.Context, x RecoveryItem) bool {
	return x.ID == p.target
}
func TestDeferredBypassContinuationCrossesBlockedPrefix(t *testing.T) {
	db, _, w := recoveryFixture(t, "deployment")
	sqlMust(t, db, "TRUNCATE deployments; INSERT INTO deployments SELECT lpad(to_hex(i),32,'0')::uuid,'"+recoveryAccount+"','WAITING_INSTALLER','provision','{\"installer_ref\":{}}',1,now()-interval '1 minute' FROM generate_series(1,901)i; INSERT INTO worker_item_failures(kind,item_id,account_id,failures,next_retry_at,last_error) SELECT 'deployment',id::text,account_id,2,now()+interval '1 hour','installer retry' FROM deployments")
	p := &selectiveBypassProbe{target: "00000000-0000-0000-0000-000000000385"}
	w.Handler = p
	cursor := ""
	submit := func(_, _ string, fn func() error) {
		if err := fn(); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.round(context.Background(), nil, submit, &cursor); err != nil {
		t.Fatal(err)
	}
	if calls(&p.recoveryProbe) != 0 || cursor == "" {
		t.Fatal("first bounded page/cursor", cursor)
	}
	if err := w.round(context.Background(), nil, submit, &cursor); err != nil {
		t.Fatal(err)
	}
	if calls(&p.recoveryProbe) != 1 {
		t.Fatal("eligible row901 starved", calls(&p.recoveryProbe))
	}
}
func TestRowLockedReconciliationPreservesHealthyAccountProgress(t *testing.T) {
	db, p, w := recoveryFixture(t, "operation")
	ctx := context.Background()
	sqlMust(t, db, "INSERT INTO accounts VALUES('00000000-0000-0000-0000-000000000004','ACTIVE'); INSERT INTO operations VALUES('00000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000004','CREATE_DROPLET','running',now()-interval '1 minute')")
	lease, ok, err := acquireRecoveryLease(ctx, db, "operation", recoveryID)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err = lease.begin(ctx, recoveryAccount); err != nil {
		t.Fatal(err)
	}
	lease.close()
	if err = w.Failures.FailChecked(ctx, "operation", recoveryID, recoveryAccount, errors.New("prior")); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE worker_item_failures SET last_error='locked' WHERE item_id=$1", recoveryID); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err = w.Once(ctx)
	if !errors.Is(err, ErrRecoveryCheckpointPending) || calls(p) != 1 || time.Since(started) > 3*time.Second {
		t.Fatal("locked row blocked healthy scan", err, calls(p), time.Since(started))
	}
}
func TestSelectivePruneFaultDoesNotBlockUnrelatedRecovery(t *testing.T) {
	db, p, w := recoveryFixture(t, "operation")
	sqlMust(t, db, "INSERT INTO operations VALUES('00000000-0000-0000-0000-000000000009','"+recoveryAccount+"','CREATE_DROPLET','succeeded',now()); INSERT INTO worker_item_failures(kind,item_id,failures,next_retry_at,last_error) VALUES('operation','00000000-0000-0000-0000-000000000009',1,now(),'00000000-0000-0000-0000-000000000009'); CREATE FUNCTION reject_prune() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF OLD.item_id='00000000-0000-0000-0000-000000000009' THEN RAISE EXCEPTION 'selective prune fault'; END IF; RETURN OLD; END$$; CREATE TRIGGER reject_prune BEFORE DELETE ON worker_item_failures FOR EACH ROW EXECUTE FUNCTION reject_prune()")
	if err := w.Once(context.Background()); err == nil || calls(p) != 1 {
		t.Fatal("prune fault hid or blocked unrelated work", err, calls(p))
	}
}

func TestReaperCursorSurvivesFailedRetryDiagnosticWrites(t *testing.T) {
	db, _, _ := recoveryFixture(t, "operation")
	ctx := context.Background()
	for _, id := range []string{"a00", "a01", "a02", "a03", "a04", "a05", "a06", "a07", "a08", "a09", "z-healthy"} {
		lease, ok, err := acquireRecoveryLease(ctx, db, "operation", id)
		if err != nil || !ok {
			t.Fatal(err)
		}
		if err = lease.begin(ctx, recoveryAccount); err != nil {
			t.Fatal(err)
		}
		lease.close()
	}
	sqlMust(t, db, "CREATE FUNCTION block_poison_completion() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.item_id LIKE 'a%' THEN PERFORM pg_sleep(0.4); RAISE EXCEPTION 'completion blocked'; END IF; RETURN NEW; END$$; CREATE TRIGGER block_poison_completion BEFORE INSERT ON worker_item_failures FOR EACH ROW EXECUTE FUNCTION block_poison_completion(); CREATE FUNCTION reject_poison_diagnostic() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'diagnostic rejected'; END$$; CREATE TRIGGER reject_poison_diagnostic BEFORE UPDATE ON worker_recovery_checkpoints FOR EACH ROW EXECUTE FUNCTION reject_poison_diagnostic()")
	cursor := pendingRecovery{}
	for i := 0; i < 3; i++ {
		if err := reconcileRecoveryCheckpoints(ctx, db, nil, "recovery", &cursor); !errors.Is(err, ErrRecoveryCheckpointPending) {
			t.Fatal(err)
		}
		var n int
		if err := db.QueryRow("SELECT count(*) FROM worker_recovery_checkpoints WHERE item_id='z-healthy'").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			if checkpointCount(t, db) != 10 {
				t.Fatal("poison checkpoint discarded")
			}
			return
		}
	}
	t.Fatal("healthy abandoned checkpoint starved behind poison prefix")
}
