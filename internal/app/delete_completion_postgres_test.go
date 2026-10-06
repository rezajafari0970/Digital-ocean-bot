package app

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
)

type deleteFixture struct {
	item droplets.LifecycleItem
	op   string
}

func newDeleteFixture(t *testing.T, db *sql.DB, state, opState string) deleteFixture {
	t.Helper()
	f := newBootstrapFixture(t, db)
	x := deleteFixture{item: droplets.LifecycleItem{ID: f.d.DropletID, AccountID: f.d.AccountID, ProviderID: "fixture-resource", State: droplets.State(state)}}
	execBootstrap(t, db, "UPDATE droplets SET state=$2,expires_at=now()-interval '2 hours' WHERE id=$1", x.item.ID, state)
	execBootstrap(t, db, "UPDATE accounts SET enabled=true,desired_server_count=1 WHERE id=$1", x.item.AccountID)
	execBootstrap(t, db, "UPDATE deployments SET provider_id=$2 WHERE droplet_id=$1", x.item.ID, x.item.ProviderID)
	execBootstrap(t, db, "INSERT INTO schedules(id,account_id,profile_id,interval_seconds,next_run_at) VALUES(gen_random_uuid(),$1,$2,60,now()+interval '1 hour')", f.d.AccountID, f.d.ProfileID)
	execBootstrap(t, db, "INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed) VALUES(gen_random_uuid(),$1,'vultr','fixture-resource','server','retiring',true)", f.d.AccountID)
	err := db.QueryRow(`INSERT INTO operations(id,account_id,kind,idempotency_key,state,resource_id,attempt)
		VALUES(gen_random_uuid(),$1,'DELETE_DROPLET',$2,$3,'fixture-resource',1) RETURNING id::text`,
		f.d.AccountID, "lifecycle-delete:"+f.d.DropletID, opState).Scan(&x.op)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func assertDeleteState(t *testing.T, db *sql.DB, x deleteFixture, wantDroplet, wantOperation string) {
	t.Helper()
	var ds, os string
	var attempts int
	if err := db.QueryRow("SELECT d.state,o.state,o.attempt FROM droplets d JOIN operations o ON o.id=$2 WHERE d.id=$1", x.item.ID, x.op).Scan(&ds, &os, &attempts); err != nil {
		t.Fatal(err)
	}
	if ds != wantDroplet || os != wantOperation || attempts != 1 {
		t.Fatalf("droplet=%s operation=%s attempts=%d", ds, os, attempts)
	}
}

func TestDeleteCompletionAtomicAndRetiringRace(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c := Container{DB: db}
	for _, state := range []string{"RETIRING", "DELETING"} {
		t.Run(state, func(t *testing.T) {
			x := newDeleteFixture(t, db, state, "verifying")
			if err := c.completeRecoveredDelete(ctx, x.op, x.item.AccountID, x.item.ProviderID, 0); err != nil {
				t.Fatal(err)
			}
			assertDeleteState(t, db, x, "DELETED", "succeeded")
			var backfill bool
			var resource, dep string
			var next time.Time
			if err := db.QueryRow(`SELECT d.backfill_required,r.state,dep.state,s.next_run_at FROM droplets d
				JOIN resources r ON r.account_id=d.account_id AND r.provider_resource_id=d.provider_resource_id
				JOIN deployments dep ON dep.droplet_id=d.id JOIN schedules s ON s.account_id=d.account_id WHERE d.id=$1`, x.item.ID).Scan(&backfill, &resource, &dep, &next); err != nil {
				t.Fatal(err)
			}
			if !backfill || resource != "deleted" || dep != "FAILED" || next.After(time.Now()) {
				t.Fatal("completion/backfill not atomic", backfill, resource, dep, next)
			}
			// Recovery can commit before the old lifecycle worker returns from DELETE.
			ok, err := (droplets.LifecycleStore{DB: db}).Transition(ctx, x.item.ID, droplets.Retiring, droplets.Deleting)
			if err != nil || ok {
				t.Fatal("late lifecycle CAS resurrected deletion", ok, err)
			}
			if err := c.completeRecoveredDelete(ctx, x.op, x.item.AccountID, x.item.ProviderID, 0); err == nil {
				t.Fatal("obsolete operation version accepted")
			}
			assertDeleteState(t, db, x, "DELETED", "succeeded")
		})
	}
}

func TestDeleteCompletionRollsBackOperationOnLocalFault(t *testing.T) {
	db := installerBootstrapDB(t)
	x := newDeleteFixture(t, db, "DELETING", "verifying")
	execBootstrap(t, db, `CREATE FUNCTION reject_delete_completion() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='DELETED' THEN RAISE EXCEPTION 'injected-completion-fault'; END IF; RETURN NEW; END; $$`)
	execBootstrap(t, db, "CREATE TRIGGER reject_delete_completion BEFORE UPDATE ON droplets FOR EACH ROW EXECUTE FUNCTION reject_delete_completion()")
	c := Container{DB: db}
	if err := c.completeRecoveredDelete(context.Background(), x.op, x.item.AccountID, x.item.ProviderID, 0); err == nil {
		t.Fatal("fault not reached")
	}
	assertDeleteState(t, db, x, "DELETING", "verifying")
	var future bool
	if err := db.QueryRow("SELECT next_run_at>now()+interval '30 minutes' FROM schedules WHERE account_id=$1", x.item.AccountID).Scan(&future); err != nil || !future {
		t.Fatal("backfill leaked from rollback", err)
	}
	execBootstrap(t, db, "DROP TRIGGER reject_delete_completion ON droplets")
	if err := c.completeRecoveredDelete(context.Background(), x.op, x.item.AccountID, x.item.ProviderID, 0); err != nil {
		t.Fatal(err)
	}
	assertDeleteState(t, db, x, "DELETED", "succeeded")
}

func TestDeleteCompletionHistoricalReplayWithoutNetwork(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c := Container{DB: db} // No secrets, provider driver, account repository or proxy runtime.
	for _, state := range []string{"RETIRING", "DELETING"} {
		t.Run(state, func(t *testing.T) {
			x := newDeleteFixture(t, db, state, "succeeded")
			var wg sync.WaitGroup
			errs := make(chan error, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); errs <- c.ProcessLifecycle(ctx, x.item) }()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			assertDeleteState(t, db, x, "DELETED", "succeeded")
			var before, after time.Time
			if err := db.QueryRow("SELECT next_run_at FROM schedules WHERE account_id=$1", x.item.AccountID).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if err := c.ProcessLifecycle(ctx, x.item); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow("SELECT next_run_at FROM schedules WHERE account_id=$1", x.item.AccountID).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if !before.Equal(after) {
				t.Fatal("duplicate receipt woke scheduler")
			}
		})
	}
}

func TestDeleteCompletionRejectsUnprovenOrMismatchedReceipt(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c := Container{DB: db}
	for _, state := range []string{"planned", "running", "unknown", "verifying", "failed"} {
		x := newDeleteFixture(t, db, "DELETING", state)
		if done, err := c.reconcileCompletedDelete(ctx, x.item); err != nil || done {
			t.Fatal("unproven outcome accepted", state, done, err)
		}
		assertDeleteState(t, db, x, "DELETING", state)
	}
	for _, kind := range []string{"provider", "account", "key", "operation_kind", "ready"} {
		t.Run(kind, func(t *testing.T) {
			x := newDeleteFixture(t, db, "DELETING", "succeeded")
			bad := x.item
			switch kind {
			case "provider":
				bad.ProviderID = "other-resource"
			case "account":
				bad.AccountID = newDeleteFixture(t, db, "DELETING", "verifying").item.AccountID
			case "key":
				execBootstrap(t, db, "UPDATE operations SET idempotency_key='unrelated' WHERE id=$1", x.op)
			case "operation_kind":
				execBootstrap(t, db, "UPDATE operations SET kind='CREATE_DROPLET' WHERE id=$1", x.op)
			case "ready":
				execBootstrap(t, db, "UPDATE droplets SET state='READY' WHERE id=$1", x.item.ID)
			}
			if done, err := c.reconcileCompletedDelete(ctx, bad); err != nil || done {
				t.Fatal("mismatch accepted", done, err)
			}
			want := "DELETING"
			if kind == "ready" {
				want = "READY"
			}
			assertDeleteState(t, db, x, want, "succeeded")
		})
	}
	x := newDeleteFixture(t, db, "READY", "verifying")
	if err := c.completeRecoveredDelete(ctx, x.op, x.item.AccountID, x.item.ProviderID, 0); err == nil {
		t.Fatal("active READY state overwritten")
	}
	assertDeleteState(t, db, x, "READY", "verifying")
}

func TestDeleteCompletionDoesNotBackfillDisabledOrReplacement(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	for _, kind := range []string{"disabled", "replacement"} {
		x := newDeleteFixture(t, db, "DELETING", "verifying")
		if kind == "disabled" {
			execBootstrap(t, db, "UPDATE accounts SET enabled=false WHERE id=$1", x.item.AccountID)
		} else {
			execBootstrap(t, db, "UPDATE droplets SET replacement_deployment_id=(SELECT id FROM deployments WHERE droplet_id=$1 LIMIT 1) WHERE id=$1", x.item.ID)
		}
		if err := c.completeRecoveredDelete(context.Background(), x.op, x.item.AccountID, x.item.ProviderID, 0); err != nil {
			t.Fatal(err)
		}
		var backfill, future bool
		if err := db.QueryRow("SELECT d.backfill_required,s.next_run_at>now()+interval '30 minutes' FROM droplets d JOIN schedules s ON s.account_id=d.account_id WHERE d.id=$1", x.item.ID).Scan(&backfill, &future); err != nil {
			t.Fatal(err)
		}
		if backfill || !future {
			t.Fatal("unexpected backfill", kind)
		}
	}
}

func TestDeleteCompletionFreshTargetMismatchRollsBack(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	x := newDeleteFixture(t, db, "DELETING", "verifying")
	execBootstrap(t, db, "UPDATE operations SET resource_id='wrong-provider-resource' WHERE id=$1", x.op)
	if err := c.completeRecoveredDelete(context.Background(), x.op, x.item.AccountID, "wrong-provider-resource", 0); err == nil {
		t.Fatal("wrong provider receipt accepted")
	}
	assertDeleteState(t, db, x, "DELETING", "verifying")
	execBootstrap(t, db, "UPDATE operations SET resource_id='fixture-resource',idempotency_key='lifecycle-delete:00000000-0000-0000-0000-000000000000' WHERE id=$1", x.op)
	if err := c.completeRecoveredDelete(context.Background(), x.op, x.item.AccountID, x.item.ProviderID, 0); err == nil {
		t.Fatal("missing lifecycle target accepted")
	}
	assertDeleteState(t, db, x, "DELETING", "verifying")
	execBootstrap(t, db, "UPDATE operations SET idempotency_key='cleanup-owned' WHERE id=$1", x.op)
	if err := c.completeRecoveredDelete(context.Background(), x.op, x.item.AccountID, x.item.ProviderID, 0); err == nil {
		t.Fatal("generic receipt accepted for lifecycle droplet")
	}
	assertDeleteState(t, db, x, "DELETING", "verifying")
}

func TestDeleteCompletionLockedDiscoveryOnlyReplaysSuccess(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c := Container{DB: db}
	for _, providerState := range []string{"LOCKED", "BILLING_BLOCKED"} {
		success := newDeleteFixture(t, db, "DELETING", "succeeded")
		pending := newDeleteFixture(t, db, "DELETING", "verifying")
		for _, x := range []deleteFixture{success, pending} {
			execBootstrap(t, db, "UPDATE accounts SET provider_state=$2 WHERE id=$1", x.item.AccountID, providerState)
		}
		due, err := (droplets.LifecycleStore{DB: db}).Due(ctx, time.Now(), 100)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range due {
			if item.ID == pending.item.ID {
				t.Fatal("blocked unconfirmed delete entered dispatch")
			}
			if item.ID == success.item.ID {
				found = true
				if err := c.ProcessLifecycle(ctx, item); err != nil {
					t.Fatal(err)
				}
			}
		}
		if !found {
			t.Fatal("completed local receipt not discoverable")
		}
		assertDeleteState(t, db, success, "DELETED", "succeeded")
		assertDeleteState(t, db, pending, "DELETING", "verifying")
	}
}

func TestDeleteCompletionOwnedResourceWithoutDroplet(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c := Container{DB: db}
	x := newDeleteFixture(t, db, "DELETING", "verifying")
	// Generic account cleanup can own a provider resource without a droplet.
	execBootstrap(t, db, "UPDATE operations SET idempotency_key='cleanup-owned',resource_id='owned-orphan' WHERE id=$1", x.op)
	execBootstrap(t, db, "INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed) VALUES(gen_random_uuid(),$1,'vultr','owned-orphan','server','retiring',true)", x.item.AccountID)
	if err := c.completeRecoveredDelete(ctx, x.op, x.item.AccountID, "owned-orphan", 0); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRow("SELECT state FROM resources WHERE account_id=$1 AND provider_resource_id='owned-orphan'", x.item.AccountID).Scan(&state); err != nil || state != "deleted" {
		t.Fatal(state, err)
	}
	assertDeleteState(t, db, x, "DELETING", "succeeded")
	y := newDeleteFixture(t, db, "DELETING", "verifying")
	execBootstrap(t, db, "UPDATE operations SET idempotency_key='cleanup-unknown',resource_id='unowned-unknown' WHERE id=$1", y.op)
	if err := c.completeRecoveredDelete(ctx, y.op, y.item.AccountID, "unowned-unknown", 0); err == nil {
		t.Fatal("unowned missing target accepted")
	}
	assertDeleteState(t, db, y, "DELETING", "verifying")
}
