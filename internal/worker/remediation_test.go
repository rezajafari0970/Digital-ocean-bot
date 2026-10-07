package worker

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/residentialsync"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestRecoveryPoisonedCheckpointPreservesHealthyAccountProgress(t *testing.T) {
	db, probe, w := recoveryFixture(t, "operation")
	ctx := context.Background()
	const otherAccount = "00000000-0000-0000-0000-000000000004"
	const otherID = "00000000-0000-0000-0000-000000000003"
	sqlMust(t, db, "INSERT INTO accounts VALUES('"+otherAccount+"','ACTIVE'); INSERT INTO operations VALUES('"+otherID+"','"+otherAccount+"','CREATE_DROPLET','running',now()-interval '1 minute')")
	lease, ok, err := acquireRecoveryLease(ctx, db, "operation", recoveryID)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err = lease.begin(ctx, recoveryAccount); err != nil {
		t.Fatal(err)
	}
	lease.close()
	sqlMust(t, db, "CREATE FUNCTION audit_poison() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.item_id='"+recoveryID+"' THEN RAISE EXCEPTION 'selective checkpoint persistence fault'; END IF; RETURN NEW; END$$; CREATE TRIGGER audit_poison BEFORE INSERT ON worker_item_failures FOR EACH ROW EXECUTE FUNCTION audit_poison()")

	if err = w.Once(ctx); !errors.Is(err, ErrRecoveryCheckpointPending) {
		t.Fatal(err)
	}
	if calls(probe) != 1 || checkpointCount(t, db) != 1 {
		t.Fatal("unrelated work blocked or checkpoint lost", calls(probe))
	}
	sqlMust(t, db, "UPDATE operations SET state='succeeded' WHERE id='"+otherID+"'")
	var after time.Time
	if err = db.QueryRow("SELECT reconcile_after FROM worker_recovery_checkpoints").Scan(&after); err != nil || time.Until(after) < 25*time.Second {
		t.Fatal(after, err)
	}
	if err = w.Once(ctx); !errors.Is(err, ErrRecoveryCheckpointPending) || calls(probe) != 1 {
		t.Fatal("delayed poison reentered handler or faked healthy", err, calls(probe))
	}
	sqlMust(t, db, "DROP TRIGGER audit_poison ON worker_item_failures; UPDATE worker_recovery_checkpoints SET reconcile_after=now()")
	if err = w.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if checkpointCount(t, db) != 0 || calls(probe) != 1 {
		t.Fatal("reconciliation replayed work")
	}

}
func TestRecoveryDueEligibilityPrecedesDiscoveryWindow(t *testing.T) {
	db, p, w := recoveryFixture(t, "operation")
	sqlMust(t, db, "TRUNCATE operations; INSERT INTO accounts SELECT md5('audit-account-'||i)::uuid,'ACTIVE' FROM generate_series(1,101)i; INSERT INTO operations SELECT md5('audit-op-'||i)::uuid,md5('audit-account-'||i)::uuid,'CREATE_DROPLET','running',now()-interval '1 minute'-(102-i)*interval '1 second' FROM generate_series(1,101)i; INSERT INTO worker_item_failures(kind,item_id,account_id,failures,next_retry_at,last_error) SELECT 'operation',md5('audit-op-'||i)::uuid::text,md5('audit-account-'||i)::uuid,6,now()+interval '60 seconds','temporary failure' FROM generate_series(1,100)i")

	if err := w.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls(p) != 1 {
		t.Fatal("runnable account101 hidden behind delayed prefix", calls(p))
	}

}
func TestResidentialPersistenceFaultPacesAndCancels(t *testing.T) {
	db := failureLedgerFixture(t)
	ctx := context.Background()
	sqlMust(t, db, `CREATE TABLE panel_instances(id uuid,droplet_id uuid,enabled bool);
 CREATE TABLE droplets(id uuid,state text,expires_at timestamptz);
 CREATE TABLE deployments(droplet_id uuid,state text);
 CREATE TABLE residential_routing_control(enabled bool,fleet bool,panel_ids uuid[],revision bigint);
 CREATE TABLE panel_routing_state(panel_id uuid,revision bigint,next_check_at timestamptz);
 INSERT INTO panel_instances VALUES('00000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000003',true);
 INSERT INTO droplets VALUES('00000000-0000-0000-0000-000000000003','READY',now()+interval '1 hour');
 INSERT INTO deployments VALUES('00000000-0000-0000-0000-000000000003','PANEL_COMPLETE');
 INSERT INTO residential_routing_control VALUES(true,true,'{}',1);
 CREATE FUNCTION audit_residential_fail() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'selective retry persistence unavailable'; END$$;
 CREATE TRIGGER audit_residential_fail BEFORE INSERT ON worker_item_failures FOR EACH ROW EXECUTE FUNCTION audit_residential_fail();`)
	svc := residentialsync.Service{DB: db}
	failures := FailureStore{DB: db}

	panel, found, err := svc.NextDuePanelShard(ctx, true, 0, 1)
	if err != nil || !found {
		t.Fatal(found, err)
	}
	// A selective reservation failure cannot monopolize discovery.
	sqlMust(t, db, "INSERT INTO panel_instances VALUES('00000000-0000-0000-0000-000000000004','00000000-0000-0000-0000-000000000003',true)")
	if claimed, err := failures.ReserveOutcome(ctx, "residential_sync", panel.ID, "", 90*time.Second); err == nil || claimed {
		t.Fatal("failed preclaim admitted native work")
	}
	next, found, err := svc.NextDuePanelShardAfter(ctx, true, 0, 1, panel.ID)
	if err != nil || !found || next.ID == panel.ID {
		t.Fatal("poison panel starves next panel", next, err)
	}
	sqlMust(t, db, "DROP TRIGGER audit_residential_fail ON worker_item_failures")
	if claimed, err := failures.ReserveOutcome(ctx, "residential_sync", panel.ID, "", 90*time.Second); err != nil || !claimed {
		t.Fatal(claimed, err)
	}
	sqlMust(t, db, "CREATE TRIGGER audit_residential_fail BEFORE INSERT ON worker_item_failures FOR EACH ROW EXECUTE FUNCTION audit_residential_fail()")
	start := time.Now()
	paced, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
	defer cancel()
	err = failures.RecordOutcome(paced, "residential_sync", panel.ID, "", errors.New("initial SQL fault"))
	if err == nil || !errors.Is(err, context.DeadlineExceeded) || time.Since(start) < 60*time.Millisecond {
		t.Fatal("persistence failure was swallowed or busy-looped", err, time.Since(start))
	}
	// Reconstruction retains the reservation after the failed outcome write.
	if claimed, err := (FailureStore{DB: db}).ReserveOutcome(ctx, "residential_sync", panel.ID, "", 90*time.Second); err == nil || claimed {
		t.Fatal("injected claim write should fail", claimed, err)
	}
	sqlMust(t, db, "DROP TRIGGER audit_residential_fail ON worker_item_failures")
	if claimed, err := (FailureStore{DB: db}).ReserveOutcome(ctx, "residential_sync", panel.ID, "", 90*time.Second); err != nil || claimed {
		t.Fatal("durable reservation lost on restart", claimed, err)
	}
	sqlMust(t, db, "DELETE FROM panel_instances WHERE id='00000000-0000-0000-0000-000000000004'")
	if err = failures.RecordOutcome(ctx, "residential_sync", panel.ID, "", errors.New("native timeout")); err != nil {
		t.Fatal(err)
	}
	_, found, err = svc.NextDuePanelShard(ctx, true, 0, 1)
	if err != nil || found {
		t.Fatal("durable delay not honored", found, err)
	}

}

func TestDeletedAccountCheckpointReconcilesWithoutPoisoningOthers(t *testing.T) {
	db, p, w := recoveryFixture(t, "operation")
	ctx := context.Background()
	sqlMust(t, db, "ALTER TABLE accounts ADD PRIMARY KEY(id); ALTER TABLE worker_item_failures ADD FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE; INSERT INTO accounts VALUES('00000000-0000-0000-0000-000000000004','ACTIVE'); INSERT INTO operations VALUES('00000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000004','CREATE_DROPLET','running',now()-interval '1 minute')")
	lease, ok, err := acquireRecoveryLease(ctx, db, "operation", recoveryID)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err = lease.begin(ctx, recoveryAccount); err != nil {
		t.Fatal(err)
	}
	lease.close()
	sqlMust(t, db, "DELETE FROM operations WHERE account_id='"+recoveryAccount+"'; DELETE FROM accounts WHERE id='"+recoveryAccount+"'")

	if err = w.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if calls(p) != 1 || checkpointCount(t, db) != 0 {
		t.Fatal("removed-account orphan blocks healthy work", calls(p))
	}

}

func TestFailureLedgerNormalizesAndBoundsDatabaseText(t *testing.T) {
	db := failureLedgerFixture(t)
	for _, msg := range []string{strings.Repeat("€", 700), strings.Repeat("خطای فارسی🙂", 400), "NUL\x00bad", string([]byte{0xff, 0xfe})} {
		if err := (FailureStore{DB: db}).FailChecked(context.Background(), "operation", "text", "", errors.New(msg)); err != nil {
			t.Fatal(err)
		}
		var saved string
		if err := db.QueryRow("SELECT last_error FROM worker_item_failures WHERE item_id='text'").Scan(&saved); err != nil {
			t.Fatal(err)
		}
		if len(saved) > 2048 || !utf8.ValidString(saved) || strings.ContainsRune(saved, 0) {
			t.Fatal("invalid database text")
		}
	}
}
func TestAccountPurgeAndRecoveryOwnershipCannotOverlap(t *testing.T) {
	db, _, _ := recoveryFixture(t, "operation")
	ctx := context.Background()
	live, ok, err := acquireRecoveryLease(ctx, db, "operation", recoveryID)
	if err != nil || !ok {
		t.Fatal(err)
	}
	defer live.close()
	if err = live.begin(ctx, recoveryAccount); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = FenceRecoveryAccountPurge(ctx, tx, recoveryAccount); !errors.Is(err, ErrRecoveryAccountBusy) {
		t.Fatal(err)
	}
	tx.Rollback()
	live.close()
	// Wait for actual SQL session release; an orphan must first reconcile.
	waitRecovery(t, func() bool {
		_ = reconcileRecoveryCheckpoints(ctx, db, nil, "recovery")
		return checkpointCount(t, db) == 0
	})
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = FenceRecoveryAccountPurge(ctx, tx, recoveryAccount); err != nil {
		t.Fatal(err)
	}
	blocked, ok, err := acquireRecoveryLease(ctx, db, "operation", "new")
	if err != nil || !ok {
		t.Fatal(err)
	}
	defer blocked.close()
	if err = blocked.begin(ctx, recoveryAccount); !errors.Is(err, ErrRecoveryAccountBusy) {
		t.Fatal("purge did not fence new recovery", err)
	}
}
func TestDeploymentDueRowsPrecedeDeferredBypassCandidates(t *testing.T) {
	db, p, w := recoveryFixture(t, "deployment")
	sqlMust(t, db, "TRUNCATE deployments; INSERT INTO accounts SELECT md5('deploy-account-'||i)::uuid,'ACTIVE' FROM generate_series(1,101)i; INSERT INTO deployments SELECT md5('deploy-'||i)::uuid,md5('deploy-account-'||i)::uuid,'CREATING','create','{}',1,now()-interval '1 minute'-(102-i)*interval '1 second' FROM generate_series(1,101)i; INSERT INTO worker_item_failures(kind,item_id,account_id,failures,next_retry_at,last_error) SELECT 'deployment',md5('deploy-'||i)::uuid::text,md5('deploy-account-'||i)::uuid,6,now()+interval '60 seconds','RECOVERY_INTERRUPTED_BEFORE_DURABLE_COMPLETION' FROM generate_series(1,100)i")
	if err := w.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls(p) != 1 || p.bypasses.Load() != 0 {
		t.Fatal("due row hidden or interruption bypassed", calls(p), p.bypasses.Load())
	}
}
func TestLifecycleDueFiltersBeforeWindowWithoutSkippingOldestAccountItem(t *testing.T) {
	db, _, _ := recoveryFixture(t, "operation")
	sqlMust(t, db, `DROP TABLE droplets; CREATE TABLE droplets(id uuid,account_id uuid,provider_resource_id text,profile_id uuid,replacement_deployment_id uuid,state text,ready_at timestamptz,created_at timestamptz,expires_at timestamptz,updated_at timestamptz);
 ALTER TABLE operations ADD resource_id text,ADD idempotency_key text;
 INSERT INTO accounts SELECT md5('life-account-'||i)::uuid,'ACTIVE' FROM generate_series(1,101)i;
 INSERT INTO droplets SELECT md5('life-'||i)::uuid,md5('life-account-'||i)::uuid,'provider-'||i,NULL,NULL,'EXPIRING',now()-interval '2 hours',now()-interval '2 hours',now()-interval '1 minute'-(102-i)*interval '1 second',now() FROM generate_series(1,101)i;
 INSERT INTO worker_item_failures(kind,item_id,account_id,failures,next_retry_at,last_error) SELECT 'lifecycle',md5('life-'||i)::uuid::text,md5('life-account-'||i)::uuid,6,now()+interval '60 seconds','delay' FROM generate_series(1,100)i;
 INSERT INTO droplets SELECT md5('life-younger')::uuid,md5('life-account-1')::uuid,'younger',NULL,NULL,'EXPIRING',now()-interval '2 hours',now()-interval '2 hours',now()-interval '1 second',now();`)
	items, err := (droplets.LifecycleStore{DB: db}).Due(context.Background(), time.Now(), 100)
	if err != nil || len(items) != 1 || items[0].ProviderID != "provider-101" {
		t.Fatal(items, err)
	}
}
