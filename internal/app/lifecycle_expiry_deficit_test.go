package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
)

type expiryDeficitFixture struct {
	old                 droplets.LifecycleItem
	sibling, panel, dep string
}

func newExpiryDeficitFixture(t *testing.T, db *sql.DB) expiryDeficitFixture {
	t.Helper()
	f := newBootstrapFixture(t, db)
	x := expiryDeficitFixture{old: droplets.LifecycleItem{ID: f.d.DropletID, AccountID: f.d.AccountID, ProviderID: "fixture-resource", State: droplets.Expiring, ProfileID: f.d.ProfileID}}
	execBootstrap(t, db, `UPDATE accounts SET provider='linode',enabled=true,provider_state='ACTIVE',runtime_status='READY',desired_server_count=10 WHERE id=$1`, x.old.AccountID)
	execBootstrap(t, db, `UPDATE droplets SET state='EXPIRING',expires_at=now()-interval '2 hours' WHERE id=$1`, x.old.ID)
	execBootstrap(t, db, `UPDATE deployments SET state='PANEL_COMPLETE',provider_id='fixture-resource' WHERE id=$1`, f.d.ID)
	var template string
	err := db.QueryRow(`INSERT INTO xui_database_templates(id,name,version,storage_path,sha256,size_bytes) VALUES(gen_random_uuid(),gen_random_uuid()::text,1,'fixture','fixture',1) RETURNING id::text`).Scan(&template)
	if err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, `INSERT INTO installers(name,version,manifest,sha256) VALUES('expiry-fixture',1,'{"capabilities":["xui_database","xui_panel"]}',repeat('e',64)) ON CONFLICT(name,version) DO NOTHING`)
	execBootstrap(t, db, `UPDATE deployment_profiles SET config=jsonb_build_object('database_template_id',$2::text,'installer_ref',jsonb_build_object('name','expiry-fixture','version',1)) WHERE id=$1`, f.d.ProfileID, template)
	execBootstrap(t, db, `INSERT INTO schedules(id,account_id,profile_id,interval_seconds) VALUES(gen_random_uuid(),$1,$2,60)`, x.old.AccountID, f.d.ProfileID)
	err = db.QueryRow(`INSERT INTO droplets(id,account_id,provider_resource_id,state,expires_at) VALUES(gen_random_uuid(),$1,'healthy','READY',now()+interval '2 hours') RETURNING id::text`, x.old.AccountID).Scan(&x.sibling)
	if err != nil {
		t.Fatal(err)
	}
	err = db.QueryRow(`INSERT INTO deployments(id,account_id,profile_id,droplet_id,provider_id,state,current_step) VALUES(gen_random_uuid(),$1,$2,$3,'healthy','PANEL_COMPLETE','done') RETURNING id::text`, x.old.AccountID, f.d.ProfileID, x.sibling).Scan(&x.dep)
	if err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, `INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed) SELECT gen_random_uuid(),account_id,'linode',provider_resource_id,'server','active',true FROM droplets WHERE account_id=$1`, x.old.AccountID)
	err = db.QueryRow(`INSERT INTO panel_instances(account_id,droplet_id,driver,base_url,auth_secret_ref) VALUES($1,$2,'sanaei','http://fixture.invalid','unused') RETURNING id::text`, x.old.AccountID, x.sibling).Scan(&x.panel)
	if err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, `INSERT INTO server_runtime_snapshots(panel_id,xui_active,checked_at) VALUES($1,true,now())`, x.panel)
	execBootstrap(t, db, `INSERT INTO panel_routing_state(panel_id,state,verified_at) VALUES($1,'APPLIED',now())`, x.panel)
	execBootstrap(t, db, `INSERT INTO user_capacity_snapshots(panel_id,inbound_id,port,target_users,active_users,deficit) VALUES($1,1,443,5,5,0)`, x.panel)
	raw, _ := json.Marshal(map[string]any{"Account": map[string]any{"Status": "active"}, "Inventory": map[string]any{"ObservedAt": time.Now().UTC(), "Servers": []map[string]any{{"ID": "fixture-resource", "State": "ready", "Ready": true}, {"ID": "healthy", "State": "ready", "Ready": true}}}})
	execBootstrap(t, db, `INSERT INTO provider_snapshots(id,account_id,provider,data,canonical) VALUES(gen_random_uuid(),$1,'linode','{}',$2)`, x.old.AccountID, string(raw))
	return x
}
func claimExpiryDeficit(db *sql.DB, item droplets.LifecycleItem) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	for _, prefix := range []string{"deployment-admission:", "lifecycle-excess:"} {
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, prefix+item.AccountID); err != nil {
			return "", err
		}
	}
	claim, err := claimExpiredDeficitRetirementTx(ctx, tx, item)
	if err != nil {
		return "", err
	}
	if claim != "" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO lifecycle_events(id,account_id,resource_id,state) VALUES(gen_random_uuid(),$1,$2,'RETIRING')`, item.AccountID, item.ID); err != nil {
			return "", err
		}
	}
	return claim, tx.Commit()
}
func TestExpiryDeficitConcurrentClaimAndDurableStart(t *testing.T) {
	db := installerBootstrapDB(t)
	x := newExpiryDeficitFixture(t, db)
	var wg sync.WaitGroup
	claims := make(chan string, 16)
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s, e := claimExpiryDeficit(db, x.old); claims <- s; errs <- e }()
	}
	wg.Wait()
	close(claims)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	claim := ""
	n := 0
	for s := range claims {
		if s != "" {
			claim = s
			n++
		}
	}
	if n != 1 {
		t.Fatalf("claims=%d", n)
	}
	var events int
	if err := db.QueryRow(`SELECT count(*) FROM lifecycle_events WHERE resource_id=$1`, x.old.ID).Scan(&events); err != nil || events != 1 {
		t.Fatal(events, err)
	}
	c := Container{DB: db}
	ctx := context.Background()
	if ok, err := c.AdmitLifecycleRetirement(ctx, x.old.AccountID, x.old.ID, x.old.ProviderID); err != nil || ok {
		t.Fatal("unstarted claim bypass", ok, err)
	}
	if ok, err := c.admitExpiredDeficitRetirement(ctx, x.old, "00000000-0000-0000-0000-000000000001"); err != nil || ok {
		t.Fatal("stale generation", ok, err)
	}
	if ok, err := c.admitExpiredDeficitRetirement(ctx, x.old, claim); err != nil || !ok {
		t.Fatal(ok, err)
	}
	// A started intent survives process loss; the ordinary deletion journal owns
	// its result, even when later health samples are missing.
	execBootstrap(t, db, `DELETE FROM user_capacity_snapshots WHERE panel_id=$1`, x.panel)
	if ok, err := c.admitExpiredDeficitRetirement(ctx, x.old, claim); err != nil || !ok {
		t.Fatal("started intent lost", ok, err)
	}
	if ok, err := c.AdmitLifecycleRetirement(ctx, x.old.AccountID, x.old.ID, x.old.ProviderID); err != nil || !ok {
		t.Fatal("ordinary identity gate", ok, err)
	}
}
func TestExpiryDeficitRejectsUnsafeProof(t *testing.T) {
	db := installerBootstrapDB(t)
	cases := map[string]string{
		"other-provider":     `UPDATE accounts SET provider='vultr' WHERE id=$1`,
		"disabled":           `UPDATE accounts SET enabled=false WHERE id=$1`,
		"provider-error":     `UPDATE accounts SET provider_error_state='denied' WHERE id=$1`,
		"desired-reached":    `UPDATE accounts SET desired_server_count=2 WHERE id=$1`,
		"expiry-extended":    `UPDATE droplets SET expires_at=now()+interval '1 hour' WHERE id=$2`,
		"unowned":            `UPDATE resources SET managed=false WHERE account_id=$1 AND provider_resource_id='fixture-resource'`,
		"sibling-unowned":    `UPDATE resources SET managed=false WHERE account_id=$1 AND provider_resource_id='healthy'`,
		"no-route":           `UPDATE schedules SET enabled=false WHERE account_id=$1`,
		"profile-off":        `UPDATE deployment_profiles SET enabled=false WHERE account_id=$1`,
		"active":             `UPDATE deployments SET state='PROVISIONING' WHERE id=$4`,
		"not-serving":        `UPDATE deployments SET state='READY' WHERE id=$4`,
		"old-panel":          `UPDATE user_capacity_snapshots SET observed_at=now()-interval '4 minutes' WHERE panel_id=$3`,
		"future-panel":       `UPDATE user_capacity_snapshots SET observed_at=now()+interval '1 minute' WHERE panel_id=$3`,
		"deficit":            `UPDATE user_capacity_snapshots SET active_users=4,deficit=1 WHERE panel_id=$3`,
		"routing":            `UPDATE panel_routing_state SET state='FAILED' WHERE panel_id=$3`,
		"guardian":           `UPDATE server_runtime_snapshots SET xui_active=false WHERE panel_id=$3`,
		"old-inventory":      `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Inventory,ObservedAt}',to_jsonb(now()-interval '5 minutes')) WHERE account_id=$1`,
		"future-inventory":   `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Inventory,ObservedAt}',to_jsonb(now()+interval '1 minute')) WHERE account_id=$1`,
		"missing-old-native": `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Inventory,Servers,0,ID}','"foreign"') WHERE account_id=$1`,
		"duplicate-native":   `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Inventory,Servers,0,ID}','"healthy"') WHERE account_id=$1`,
		"native-not-ready":   `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Inventory,Servers,1,Ready}','false') WHERE account_id=$1`,
		"unknown-create":     `INSERT INTO operations(id,account_id,kind,idempotency_key,state,resource_id) VALUES(gen_random_uuid(),$1,'CREATE_DROPLET','unknown-create','unknown','bound-id')`,
		"unknown-delete":     `INSERT INTO operations(id,account_id,kind,idempotency_key,state,resource_id) VALUES(gen_random_uuid(),$1,'DELETE_DROPLET','unknown-delete','unknown','bound-id')`,
	}
	for name, query := range cases {
		t.Run(name, func(t *testing.T) {
			x := newExpiryDeficitFixture(t, db)
			// The CTE types every positional parameter, including unused fixture IDs.
			query = `WITH args AS (SELECT $1::uuid a,$2::uuid d,$3::uuid p,$4::uuid dep) ` + query
			execBootstrap(t, db, query, x.old.AccountID, x.old.ID, x.panel, x.dep)
			if claim, err := claimExpiryDeficit(db, x.old); err != nil || claim != "" {
				t.Fatal("unsafe claim", claim, err)
			}
		})
	}
}
func TestExpiryDeficitRestartRevalidatesAndCancelsRevocation(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c := Container{DB: db}
	for _, kind := range []string{"stale", "expiry", "ownership", "disabled", "schedule", "error"} {
		t.Run(kind, func(t *testing.T) {
			x := newExpiryDeficitFixture(t, db)
			claim, err := claimExpiryDeficit(db, x.old)
			if err != nil || claim == "" {
				t.Fatal(claim, err)
			}
			switch kind {
			case "stale":
				execBootstrap(t, db, `UPDATE server_runtime_snapshots SET checked_at=now()-interval '1 hour' WHERE panel_id=$1`, x.panel)
			case "expiry":
				execBootstrap(t, db, `UPDATE droplets SET expires_at=now()+interval '1 hour' WHERE id=$1`, x.old.ID)
			case "ownership":
				execBootstrap(t, db, `UPDATE resources SET managed=false WHERE account_id=$1 AND provider_resource_id='fixture-resource'`, x.old.AccountID)
			case "disabled":
				execBootstrap(t, db, `UPDATE accounts SET enabled=false WHERE id=$1`, x.old.AccountID)
			case "schedule":
				execBootstrap(t, db, `UPDATE schedules SET enabled=false WHERE account_id=$1`, x.old.AccountID)
			case "error":
				execBootstrap(t, db, `UPDATE accounts SET provider_error_state='denied' WHERE id=$1`, x.old.AccountID)
			}
			if ok, err := c.admitExpiredDeficitRetirement(ctx, x.old, claim); err != nil || ok {
				t.Fatal("restart bypass", ok, err)
			}
			var started bool
			if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM lifecycle_deficit_claims WHERE droplet_id=$1 AND started_at IS NOT NULL)`, x.old.ID).Scan(&started); err != nil || started {
				t.Fatal(started, err)
			}
			if kind == "expiry" || kind == "ownership" || kind == "disabled" {
				var state string
				if err := db.QueryRow(`SELECT state FROM droplets WHERE id=$1`, x.old.ID).Scan(&state); err != nil || state != "EXPIRING" {
					t.Fatal(state, err)
				}
			}
		})
	}
}
func TestExpiryDeficitClaimEventFaultRollsBack(t *testing.T) {
	db := installerBootstrapDB(t)
	x := newExpiryDeficitFixture(t, db)
	execBootstrap(t, db, `CREATE FUNCTION reject_expiry_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'event fault'; END; $$`)
	execBootstrap(t, db, `CREATE TRIGGER reject_expiry_event BEFORE INSERT ON lifecycle_events FOR EACH ROW EXECUTE FUNCTION reject_expiry_event()`)
	if _, err := claimExpiryDeficit(db, x.old); err == nil {
		t.Fatal("injected failure not reached")
	}
	var state string
	var count int
	if err := db.QueryRow(`SELECT state,(SELECT count(*) FROM lifecycle_deficit_claims WHERE droplet_id=$1) FROM droplets WHERE id=$1`, x.old.ID).Scan(&state, &count); err != nil || state != "EXPIRING" || count != 0 {
		t.Fatal(state, count, err)
	}
}

func TestExpiryDeficitBackfillMustServeBeforeNextRetirement(t *testing.T) {
	db := installerBootstrapDB(t)
	x := newExpiryDeficitFixture(t, db)
	ctx := context.Background()
	c := Container{DB: db}
	claim, err := claimExpiryDeficit(db, x.old)
	if err != nil || claim == "" {
		t.Fatal(claim, err)
	}
	if ok, err := c.admitExpiredDeficitRetirement(ctx, x.old, claim); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := c.ConfirmDeleted(ctx, x.old.AccountID, x.old.ProviderID); err != nil {
		t.Fatal(err)
	}
	var nextID string
	if err := db.QueryRow(`INSERT INTO droplets(id,account_id,provider_resource_id,state,expires_at) VALUES(gen_random_uuid(),$1,'next-expired','EXPIRING',now()-interval '1 hour') RETURNING id::text`, x.old.AccountID).Scan(&nextID); err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, `INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed) VALUES(gen_random_uuid(),$1,'linode','next-expired','server','active',true)`, x.old.AccountID)
	execBootstrap(t, db, `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Inventory,Servers}',(canonical->'Inventory'->'Servers')||'[{"ID":"next-expired","Ready":true,"State":"ready"}]'::jsonb) WHERE account_id=$1`, x.old.AccountID)
	next := droplets.LifecycleItem{ID: nextID, AccountID: x.old.AccountID, ProviderID: "next-expired", State: droplets.Expiring}
	blocked := func() {
		t.Helper()
		if cl, err := claimExpiryDeficit(db, next); err != nil || cl != "" {
			t.Fatal("debt did not block", cl, err)
		}
	}
	blocked()
	// Assignment alone is not recovery. Exercise terminal failure before the
	// consistency repair can reset backfill_required, then incomplete service.
	execBootstrap(t, db, `UPDATE droplets SET replacement_deployment_id=$2,backfill_required=false WHERE id=$1`, x.old.ID, x.dep)
	execBootstrap(t, db, `UPDATE deployments SET state='FAILED' WHERE id=$1`, x.dep)
	blocked()
	execBootstrap(t, db, `UPDATE deployments SET state='READY' WHERE id=$1`, x.dep)
	blocked()
	execBootstrap(t, db, `UPDATE deployments SET state='PANEL_COMPLETE' WHERE id=$1`, x.dep)
	execBootstrap(t, db, `UPDATE user_capacity_snapshots SET active_users=4,deficit=1 WHERE panel_id=$1`, x.panel)
	blocked()
	execBootstrap(t, db, `UPDATE user_capacity_snapshots SET active_users=5,deficit=0 WHERE panel_id=$1`, x.panel)
	cl, err := claimExpiryDeficit(db, next)
	if err != nil || cl == "" {
		t.Fatal("verified refill did not release debt", cl, err)
	}
	var recovered bool
	if err := db.QueryRow(`SELECT recovered_at IS NOT NULL FROM lifecycle_deficit_claims WHERE droplet_id=$1`, x.old.ID).Scan(&recovered); err != nil || !recovered {
		t.Fatal(recovered, err)
	}
}

func TestExpiryDeficitParentLockFencesNewEvidenceAndOperations(t *testing.T) {
	db := installerBootstrapDB(t)
	x := newExpiryDeficitFixture(t, db)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT id::text FROM accounts WHERE id=$1 FOR UPDATE`, x.old.AccountID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	queries := []string{
		`INSERT INTO operations(id,account_id,kind,idempotency_key,state) VALUES(gen_random_uuid(),$1,'CREATE_DROPLET','concurrent','unknown')`,
		`INSERT INTO provider_snapshots(id,account_id,provider,data,canonical) VALUES(gen_random_uuid(),$1,'linode','{}','{"Account":{"Status":"disabled"}}')`,
		`INSERT INTO deployments(id,account_id,profile_id,state,current_step) SELECT gen_random_uuid(),$1,id,'PLANNED','create' FROM deployment_profiles WHERE account_id=$1 LIMIT 1`,
	}
	for _, query := range queries {
		c, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		start := time.Now()
		_, err := db.ExecContext(c, query, x.old.AccountID)
		deadline := c.Err()
		cancel()
		if err == nil || !errors.Is(deadline, context.DeadlineExceeded) || time.Since(start) < 150*time.Millisecond {
			t.Fatal("expected deadline while FK parent lock held", err, deadline)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	// The same three SQL shapes must succeed with valid fixtures after unlock.
	probe, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range queries {
		if _, err := probe.ExecContext(ctx, query, x.old.AccountID); err != nil {
			probe.Rollback()
			t.Fatal("unlocked insert invalid", err)
		}
	}
	if err := probe.Rollback(); err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, `INSERT INTO provider_snapshots(id,account_id,provider,data,canonical) VALUES(gen_random_uuid(),$1,'linode','{}','{"Account":{"Status":"disabled"}}')`, x.old.AccountID)
	if cl, err := claimExpiryDeficit(db, x.old); err != nil || cl != "" {
		t.Fatal("latest contradictory evidence ignored", cl, err)
	}
}

func TestExpiryDeficitCancelledGenerationCannotStartReclaimedTarget(t *testing.T) {
	db := installerBootstrapDB(t)
	x := newExpiryDeficitFixture(t, db)
	c := Container{DB: db}
	ctx := context.Background()
	old, err := claimExpiryDeficit(db, x.old)
	if err != nil || old == "" {
		t.Fatal(old, err)
	}
	execBootstrap(t, db, `UPDATE droplets SET expires_at=now()+interval '1 hour' WHERE id=$1`, x.old.ID)
	if ok, err := c.admitExpiredDeficitRetirement(ctx, x.old, old); err != nil || ok {
		t.Fatal(ok, err)
	}
	execBootstrap(t, db, `UPDATE droplets SET expires_at=now()-interval '1 hour' WHERE id=$1`, x.old.ID)
	fresh, err := claimExpiryDeficit(db, x.old)
	if err != nil || fresh == "" || fresh == old {
		t.Fatal(fresh, err)
	}
	if ok, err := c.admitExpiredDeficitRetirement(ctx, x.old, old); err != nil || ok {
		t.Fatal("obsolete generation admitted", ok, err)
	}
	if ok, err := c.admitExpiredDeficitRetirement(ctx, x.old, fresh); err != nil || !ok {
		t.Fatal("fresh generation blocked", ok, err)
	}
}

func TestExpiryDeficitIdentityRevocationRetainsFence(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c := Container{DB: db}
	for _, kind := range []string{"provider", "account"} {
		t.Run(kind, func(t *testing.T) {
			x := newExpiryDeficitFixture(t, db)
			claim, err := claimExpiryDeficit(db, x.old)
			if err != nil || claim == "" {
				t.Fatal(claim, err)
			}
			currentAccount, currentProvider := x.old.AccountID, x.old.ProviderID
			if kind == "provider" {
				currentProvider = "changed-identity"
				execBootstrap(t, db, `UPDATE droplets SET provider_resource_id=$2 WHERE id=$1`, x.old.ID, currentProvider)
			} else {
				other := newExpiryDeficitFixture(t, db)
				currentAccount = other.old.AccountID
				// Move its fixture identity aside; retain the owned inventory for this target.
				execBootstrap(t, db, `UPDATE droplets SET provider_resource_id='other-original' WHERE id=$1`, other.old.ID)
				execBootstrap(t, db, `UPDATE droplets SET account_id=$2 WHERE id=$1`, x.old.ID, currentAccount)
			}
			if kind == "provider" {
				execBootstrap(t, db, `INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed) VALUES(gen_random_uuid(),$1,'linode',$2,'server','active',true)`, currentAccount, currentProvider)
			}
			if ok, err := c.admitExpiredDeficitRetirement(ctx, x.old, claim); err != nil || ok {
				t.Fatal("changed identity admitted", ok, err)
			}
			var retained bool
			var events int
			var state string
			if err := db.QueryRow(`SELECT d.state,EXISTS(SELECT 1 FROM lifecycle_deficit_claims cl WHERE cl.droplet_id=d.id AND cl.claim_id=$2 AND cl.started_at IS NULL),(SELECT count(*) FROM lifecycle_events e WHERE e.resource_id=d.id AND e.state='EXPIRING') FROM droplets d WHERE d.id=$1`, x.old.ID, claim).Scan(&state, &retained, &events); err != nil || state != "RETIRING" || !retained || events != 0 {
				t.Fatal(state, retained, events, err)
			}
			if ok, err := c.AdmitLifecycleRetirement(ctx, currentAccount, x.old.ID, currentProvider); err != nil || ok {
				t.Fatal("ordinary admission lost identity fence", ok, err)
			}
		})
	}
}

func TestExpiryDeficitRecoveryRecordedBeforeNextExpiry(t *testing.T) {
	db := installerBootstrapDB(t)
	x := newExpiryDeficitFixture(t, db)
	c := Container{DB: db}
	ctx := context.Background()
	claim, err := claimExpiryDeficit(db, x.old)
	if err != nil || claim == "" {
		t.Fatal(claim, err)
	}
	if ok, err := c.admitExpiredDeficitRetirement(ctx, x.old, claim); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := c.ConfirmDeleted(ctx, x.old.AccountID, x.old.ProviderID); err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, `UPDATE droplets SET replacement_deployment_id=$2,backfill_required=false WHERE id=$1`, x.old.ID, x.dep)
	// A deleted old provider ID need not remain in native inventory.
	execBootstrap(t, db, `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Inventory,Servers}','[{"ID":"healthy","State":"ready","Ready":true}]') WHERE account_id=$1`, x.old.AccountID)
	execBootstrap(t, db, `UPDATE user_capacity_snapshots SET deficit=1,active_users=4 WHERE panel_id=$1`, x.panel)
	if err := c.reconcileExpiryDeficitBackfill(ctx); err != nil {
		t.Fatal(err)
	}
	var at sql.NullTime
	if err := db.QueryRow(`SELECT recovered_at FROM lifecycle_deficit_claims WHERE droplet_id=$1`, x.old.ID).Scan(&at); err != nil || at.Valid {
		t.Fatal("incomplete service closed debt", at, err)
	}
	execBootstrap(t, db, `UPDATE user_capacity_snapshots SET deficit=0,active_users=5 WHERE panel_id=$1`, x.panel)
	if err := c.reconcileExpiryDeficitBackfill(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT recovered_at FROM lifecycle_deficit_claims WHERE droplet_id=$1`, x.old.ID).Scan(&at); err != nil || !at.Valid {
		t.Fatal("recovery not recorded", at, err)
	}
	execBootstrap(t, db, `UPDATE droplets SET state='EXPIRING',expires_at=now()-interval '1 minute' WHERE id=$1`, x.sibling)
	if err := c.reconcileExpiryDeficitBackfill(ctx); err != nil {
		t.Fatal(err)
	}
	var after time.Time
	if err := db.QueryRow(`SELECT recovered_at FROM lifecycle_deficit_claims WHERE droplet_id=$1`, x.old.ID).Scan(&after); err != nil || !after.Equal(at.Time) {
		t.Fatal("completed debt reopened", at, after, err)
	}
}
