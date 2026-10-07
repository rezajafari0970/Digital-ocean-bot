package app

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
)

func capacityRecoveryFixture(t *testing.T) *sql.DB {
	t.Helper()
	db := fenceTestDB(t)
	_, err := db.Exec(`ALTER TABLE accounts ADD COLUMN provider_state text DEFAULT 'ACTIVE',ADD COLUMN provider_error_state text,ADD COLUMN desired_server_count int DEFAULT 5,ADD COLUMN runtime_status text,ADD COLUMN runtime_status_detail text,ADD COLUMN runtime_status_at timestamptz,ADD COLUMN updated_at timestamptz DEFAULT now();
 CREATE TABLE account_create_blocks(account_id text,version bigint,code text,blocked_at timestamptz);
 CREATE TABLE provider_snapshots(account_id text,provider text,canonical jsonb,data jsonb,created_at timestamptz);
 CREATE TABLE operations(account_id text,kind text,resource_id text,state text);
 CREATE TABLE deployments(account_id text,state text,droplet_id text,updated_at timestamptz DEFAULT now());
 CREATE TABLE resources(account_id text,provider_resource_id text,managed boolean,state text,updated_at timestamptz DEFAULT now());
 CREATE TABLE lifecycle_events(id uuid,account_id text,resource_id text,state text);
 CREATE TABLE account_rule_application(account_id text,waiting_droplet_id text,retirement_started boolean,updated_at timestamptz);
 CREATE TABLE droplets(id text PRIMARY KEY,provider_resource_id text,account_id text,state text,replacement_deployment_id text,backfill_required boolean DEFAULT false,expires_at timestamptz,created_at timestamptz DEFAULT now(),updated_at timestamptz DEFAULT now());
 INSERT INTO provider_snapshots VALUES('test','upcloud','{"Capacity":{"ComputeLimit":2,"LimitKnown":true,"ComputeInUse":2},"Account":{"Status":"active"}}','{}',now());
 INSERT INTO droplets(id,account_id,state,expires_at) VALUES('old','test','EXPIRING',now()-interval '2 hours'),('new','test','EXPIRING',now()-interval '1 hour');
 UPDATE droplets SET provider_resource_id=id;
 INSERT INTO resources(account_id,provider_resource_id,managed,state) SELECT account_id,id,true,'active' FROM droplets;`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}
func claimRecovery(t *testing.T, db *sql.DB, id string) (bool, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	for _, prefix := range []string{"deployment-admission:", "lifecycle-excess:"} {
		if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", prefix+"test"); err != nil {
			return false, err
		}
	}
	claimed, err := claimCapacityRetirementTx(ctx, tx, droplets.LifecycleItem{ID: id, AccountID: "test", ProviderID: id, State: droplets.Expiring})
	if err != nil {
		return false, err
	}
	if claimed {
		if _, err = tx.ExecContext(ctx, `INSERT INTO lifecycle_events(id,account_id,resource_id,state) VALUES(gen_random_uuid(),'test',$1,'RETIRING')`, id); err != nil {
			return false, err
		}
	}
	return claimed, tx.Commit()
}
func TestCapacityRecoveryExactlyOneOldestAcrossConcurrentWorkers(t *testing.T) {
	db := capacityRecoveryFixture(t)
	var wg sync.WaitGroup
	results := make(chan bool, 16)
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		id := "old"
		if i%2 != 0 {
			id = "new"
		}
		wg.Add(1)
		go func() { defer wg.Done(); ok, err := claimRecovery(t, db, id); results <- ok; errs <- err }()
	}
	wg.Wait()
	close(results)
	close(errs)
	n := 0
	for ok := range results {
		if ok {
			n++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n != 1 {
		t.Fatalf("retirements=%d", n)
	}
	var state string
	var desired int
	if err := db.QueryRow("SELECT state FROM droplets WHERE id='old'").Scan(&state); err != nil || state != "RETIRING" {
		t.Fatal(state, err)
	}
	if err := db.QueryRow("SELECT desired_server_count FROM accounts").Scan(&desired); err != nil || desired != 5 {
		t.Fatal(desired, err)
	}
	if _, err := db.Exec("UPDATE droplets SET state='DELETED',backfill_required=true,updated_at=now() WHERE id='old'"); err != nil {
		t.Fatal(err)
	}
	if ok, err := claimRecovery(t, db, "new"); err != nil || ok {
		t.Fatal("second delete before backfill", ok, err)
	}
	if _, err := db.Exec("UPDATE droplets SET backfill_required=false WHERE id='old'"); err != nil {
		t.Fatal(err)
	}
	if ok, err := claimRecovery(t, db, "new"); err != nil || ok {
		t.Fatal("reused pre-delete full snapshot", ok, err)
	}
}
func TestCapacityRecoveryNeverDeletesWithoutProof(t *testing.T) {
	cases := map[string]string{
		"ownership removed": "UPDATE resources SET managed=false",
		"expiry disabled":   "UPDATE droplets SET expires_at=NULL",
		"future snapshot":   "UPDATE provider_snapshots SET created_at=now()+interval '5 minutes'",
		"disabled":          "UPDATE accounts SET enabled=false",
		"provider error":    "UPDATE accounts SET provider_error_state='BILLING_BLOCKED'",
		"zero desired":      "UPDATE accounts SET desired_server_count=0",
		"at desired":        "UPDATE accounts SET desired_server_count=2",
		"unknown limit":     `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Capacity,LimitKnown}','false')`,
		"zero limit":        `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Capacity,ComputeLimit}','0')`,
		"free slot":         `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Capacity,ComputeInUse}','1')`,
		"stale":             "UPDATE provider_snapshots SET created_at=now()-interval '3 minutes'",
		"missing":           "DELETE FROM provider_snapshots",
		"retiring":          "UPDATE droplets SET state='DELETING' WHERE id='new'",
		"pending create":    "INSERT INTO operations VALUES('test','CREATE_DROPLET','','unknown')",
		"active deployment": "INSERT INTO deployments(account_id,state) VALUES('test','PROVISIONING')",
		"backfill":          "UPDATE droplets SET state='DELETED',backfill_required=true WHERE id='new'",
		"not expired":       "UPDATE droplets SET expires_at=now()+interval '1 hour'",
		"owned replacement": "UPDATE droplets SET replacement_deployment_id='reserved' WHERE id='old'",
		"create block":      "INSERT INTO account_create_blocks VALUES('test',1,'DENIED',now())",
	}
	for name, q := range cases {
		t.Run(name, func(t *testing.T) {
			db := capacityRecoveryFixture(t)
			if _, err := db.Exec(q); err != nil {
				t.Fatal(err)
			}
			if ok, err := claimRecovery(t, db, "old"); err != nil || ok {
				t.Fatal(ok, err)
			}
		})
	}
}
func TestCapacityRecoveryDatabaseFailureDoesNotAuthorizeRetirement(t *testing.T) {
	db := capacityRecoveryFixture(t)
	if _, err := db.Exec("DROP TABLE operations"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := claimRecovery(t, db, "old"); ok {
		t.Fatal("database failure authorized retirement")
	}
	var state string
	if err := db.QueryRow("SELECT state FROM droplets WHERE id='old'").Scan(&state); err != nil || state != "EXPIRING" {
		t.Fatal(state, err)
	}
}

func TestExpiryAuthorizationRechecksStaleDiscovery(t *testing.T) {
	for _, q := range []string{
		"UPDATE droplets SET expires_at=now()+interval '1 hour' WHERE id='old'",
		"UPDATE droplets SET expires_at=NULL WHERE id='old'",
		"UPDATE accounts SET enabled=false",
		"UPDATE resources SET managed=false",
		"UPDATE droplets SET state='READY' WHERE id='old'",
	} {
		db := capacityRecoveryFixture(t)
		item := droplets.LifecycleItem{ID: "old", AccountID: "test", ProviderID: "old", State: droplets.Expiring}
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
		if ok, err := (Container{DB: db}).transitionExpiredLifecycle(context.Background(), item, droplets.Retiring); err != nil || ok {
			t.Fatal(q, ok, err)
		}
	}
}
func TestExpiryAuthorizationTransitionRollsBackOnJournalFailure(t *testing.T) {
	db := capacityRecoveryFixture(t)
	if _, err := db.Exec("DROP TABLE lifecycle_events"); err != nil {
		t.Fatal(err)
	}
	item := droplets.LifecycleItem{ID: "old", AccountID: "test", ProviderID: "old", State: droplets.Expiring}
	if ok, err := (Container{DB: db}).transitionExpiredLifecycle(context.Background(), item, droplets.Retiring); err == nil || ok {
		t.Fatal(ok, err)
	}
	var state string
	if err := db.QueryRow("SELECT state FROM droplets WHERE id='old'").Scan(&state); err != nil || state != "EXPIRING" {
		t.Fatal(state, err)
	}
}

func TestCapacityRecoveryAndProviderLockRollBackWithoutEvent(t *testing.T) {
	db := capacityRecoveryFixture(t)
	if _, err := db.Exec("DROP TABLE lifecycle_events"); err != nil {
		t.Fatal(err)
	}
	if ok, err := claimRecovery(t, db, "old"); err == nil || ok {
		t.Fatal("capacity claim escaped failed event", ok, err)
	}
	if _, err := db.Exec("UPDATE accounts SET provider_state='LOCKED'"); err != nil {
		t.Fatal(err)
	}
	item := droplets.LifecycleItem{ID: "old", AccountID: "test", ProviderID: "old", State: droplets.Expiring}
	if ok, err := (Container{DB: db}).transitionLockedProviderLifecycle(context.Background(), item); err == nil || ok {
		t.Fatal(ok, err)
	}
	var state, resource string
	if err := db.QueryRow("SELECT d.state,r.state FROM droplets d JOIN resources r ON r.provider_resource_id=d.id WHERE d.id='old'").Scan(&state, &resource); err != nil || state != "EXPIRING" || resource != "active" {
		t.Fatal(state, resource, err)
	}
}

func TestExpiryAuthorizationRejectsChangedProviderTarget(t *testing.T) {
	db := capacityRecoveryFixture(t)
	item := droplets.LifecycleItem{ID: "old", AccountID: "test", ProviderID: "old", State: droplets.Expiring}
	if _, err := db.Exec("UPDATE droplets SET provider_resource_id='new' WHERE id='old'"); err != nil {
		t.Fatal(err)
	}
	c := Container{DB: db}
	if ok, err := c.transitionExpiredLifecycle(context.Background(), item, droplets.Retiring); err != nil || ok {
		t.Fatal("stale target expiry", ok, err)
	}
	if _, err := db.Exec("UPDATE accounts SET provider_state='LOCKED'"); err != nil {
		t.Fatal(err)
	}
	if ok, err := c.transitionLockedProviderLifecycle(context.Background(), item); err != nil || ok {
		t.Fatal("stale target provider lock", ok, err)
	}
	if _, err := db.Exec("UPDATE droplets SET state='RETIRING' WHERE id='old'"); err != nil {
		t.Fatal(err)
	}
	if ok, err := c.AdmitLifecycleRetirement(context.Background(), "test", "old", "old"); err != nil || ok {
		t.Fatal("stale deletion target admitted", ok, err)
	}
}

func TestExpiryAuthorizationUnownedSiblingDoesNotAuthorizeDesiredRetirement(t *testing.T) {
	db := capacityRecoveryFixture(t)
	_, err := db.Exec(`UPDATE accounts SET desired_server_count=2;
 UPDATE resources SET managed=false WHERE provider_resource_id='new';
 UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Capacity,ComputeInUse}','1');`)
	if err != nil {
		t.Fatal(err)
	}
	item := droplets.LifecycleItem{ID: "old", AccountID: "test", ProviderID: "old", State: droplets.Expiring, ProfileID: "profile"}
	if err := (Container{DB: db}).ProcessLifecycle(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRow("SELECT state FROM droplets WHERE id='old'").Scan(&state); err != nil || state != "EXPIRING" {
		t.Fatal(state, err)
	}
}
func TestExpiryAuthorizationLockedIntentWaitsForProviderThenIsRediscovered(t *testing.T) {
	db := installerBootstrapDB(t)
	x := newDeleteFixture(t, db, "EXPIRING", "planned")
	execBootstrap(t, db, "UPDATE accounts SET provider_state='LOCKED' WHERE id=$1", x.item.AccountID)
	c := Container{DB: db}
	ctx := context.Background()
	if ok, err := c.transitionLockedProviderLifecycle(ctx, x.item); err != nil || !ok {
		t.Fatal(ok, err)
	}
	check := func(want bool) {
		t.Helper()
		items, err := (droplets.LifecycleStore{DB: db}).Due(ctx, time.Now(), 100)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, i := range items {
			if i.ID == x.item.ID {
				found = true
			}
		}
		if found != want {
			t.Fatal("provider permission discovery", found, want)
		}
	}
	check(false) // Deliberate provider permission policy, not a missing durable intent.
	execBootstrap(t, db, "UPDATE accounts SET provider_state='ACTIVE' WHERE id=$1", x.item.AccountID)
	check(true) // A crash after the transition does not erase recoverable work.
}

func TestExpiryAuthorizationActiveReservationsBlockInitialRetirement(t *testing.T) {
	for _, q := range []string{
		"INSERT INTO deployments(account_id,state) VALUES('test','RESERVED')",
		"INSERT INTO deployments(account_id,state,droplet_id) VALUES('test','PROVISIONING','new')",
	} {
		db := capacityRecoveryFixture(t)
		if _, err := db.Exec("UPDATE accounts SET desired_server_count=2"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
		item := droplets.LifecycleItem{ID: "old", AccountID: "test", ProviderID: "old", State: droplets.Expiring, ProfileID: "profile"}
		if err := (Container{DB: db}).ProcessLifecycle(context.Background(), item); err != nil {
			t.Fatal(err)
		}
		var state string
		var events int
		if err := db.QueryRow("SELECT state FROM droplets WHERE id='old'").Scan(&state); err != nil || state != "EXPIRING" {
			t.Fatal(state, err)
		}
		if err := db.QueryRow("SELECT count(*) FROM lifecycle_events").Scan(&events); err != nil || events != 0 {
			t.Fatal(events, err)
		}
	}
}
func TestExpiryAuthorizationBlockedProviderNeverEntersNetworkRuntime(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	ctx := context.Background()
	for _, providerState := range []string{"LOCKED", "BILLING_BLOCKED"} {
		for _, state := range []string{"READY", "EXPIRING", "RETIRING", "DELETING"} {
			x := newDeleteFixture(t, db, state, "planned")
			execBootstrap(t, db, "UPDATE accounts SET provider_state=$2 WHERE id=$1", x.item.AccountID, providerState)
			// Container intentionally has no network/account/secret runtime at all.
			if err := c.ProcessLifecycle(ctx, x.item); err != nil {
				t.Fatal(providerState, state, err)
			}
			execBootstrap(t, db, "UPDATE droplets SET state='RETIRING' WHERE id=$1", x.item.ID)
			if ok, err := c.AdmitLifecycleRetirement(ctx, x.item.AccountID, x.item.ID, x.item.ProviderID); err != nil || ok {
				t.Fatal("stale discovery admitted", ok, err)
			}
			execBootstrap(t, db, "UPDATE accounts SET provider_state='ACTIVE' WHERE id=$1", x.item.AccountID)
			if ok, err := c.AdmitLifecycleRetirement(ctx, x.item.AccountID, x.item.ID, x.item.ProviderID); err != nil || !ok {
				t.Fatal("permission restoration not recoverable", ok, err)
			}
		}
	}
}
