package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAccountHealthCadenceAndActualPanelEvidence(t *testing.T) {
	db := adminTestDB(t)
	a, d, p := seedPanel(t, db, "http://fixture.invalid")
	sqlMust(t, db, "UPDATE accounts SET enabled=true,provider_checked_at=now()-interval '4 minutes',provider_inventory_interval_seconds=300 WHERE id=$1", a)
	sqlMust(t, db, `INSERT INTO provider_snapshots(id,account_id,provider,data,canonical,created_at)
 SELECT gen_random_uuid(),$1,'digitalocean','{}',jsonb_build_object('Account',jsonb_build_object('Status','active'),'Inventory',jsonb_build_object('Servers',jsonb_build_array(jsonb_build_object('ID',provider_resource_id,'State','ready')))),now()-interval '4 minutes' FROM droplets WHERE id=$2`, a, d)
	sqlMust(t, db, "INSERT INTO panel_routing_state(panel_id,state) VALUES($1,'APPLIED')", p)
	sqlMust(t, db, "INSERT INTO panel_inventory_syncs(panel_id,state,finished_at) VALUES($1,'COMPLETED',now())", p)
	check := func(active int, reason string) {
		t.Helper()
		var raw []byte
		if e := db.QueryRow(dashboardCountsSQL).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		var v struct {
			Active  int `json:"active_servers"`
			Details []struct {
				Reason string `json:"reason"`
			} `json:"server_health"`
		}
		if e := json.Unmarshal(raw, &v); e != nil {
			t.Fatal(e)
		}
		if v.Active != active {
			t.Fatal(string(raw))
		}
		if reason != "" && (len(v.Details) != 1 || v.Details[0].Reason != reason) {
			t.Fatal(string(raw))
		}
	}
	check(1, "")
	// Strict two-minute build admission remains independent of display cadence.
	state, e := (&Server{DB: db}).accountBuildState(context.Background(), a)
	if e != nil || state.ProviderCanCreate || state.ProviderFreshness != "scheduled" {
		t.Fatal(state, e)
	}
	sqlMust(t, db, "UPDATE accounts SET provider_inventory_interval_seconds=90 WHERE id=$1", a)
	check(0, "PROVIDER_OBSERVATION_STALE")
	sqlMust(t, db, "UPDATE accounts SET provider_inventory_interval_seconds=300 WHERE id=$1", a)
	sqlMust(t, db, "UPDATE provider_snapshots SET created_at=now()-interval '6 minutes'")
	check(0, "PROVIDER_OBSERVATION_STALE")
	sqlMust(t, db, "UPDATE provider_snapshots SET created_at=now()+interval '1 minute'")
	check(0, "PROVIDER_OBSERVATION_STALE")
	sqlMust(t, db, "UPDATE provider_snapshots SET created_at=now()")
	sqlMust(t, db, "UPDATE panel_inventory_syncs SET finished_at=now()-interval '3 minutes'")
	check(0, "PANEL_OBSERVATION_STALE")
	sqlMust(t, db, "UPDATE panel_inventory_syncs SET state='FAILED',finished_at=now()")
	check(0, "PANEL_SYNC_FAILED")
	sqlMust(t, db, "UPDATE panel_inventory_syncs SET state='COMPLETED'")
	sqlMust(t, db, "UPDATE accounts SET provider_error_state='TRANSPORT_ERROR' WHERE id=$1", a)
	check(0, "PROVIDER_UNAVAILABLE")
	sqlMust(t, db, "UPDATE droplets SET state='PROVISIONING' WHERE id=$1", d)
	check(0, "PROVISIONING")
	sqlMust(t, db, "UPDATE droplets SET state='RETIRING' WHERE id=$1", d)
	check(0, "PENDING_DELETION")
}

func TestAccountCapacityWarningClassification(t *testing.T) {
	db := adminTestDB(t)
	a := seedBuildAccount(t, db, "digitalocean")
	putBuildSnapshot(t, db, a, "warning", true, 3, 3)
	message := "Your team has created the maximum allowed number of Droplets. Please resolve this on the control panel."
	sqlMust(t, db, `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Account,StatusMessage}',to_jsonb($1::text),true)`, message)
	s := Server{DB: db}
	v, e := s.accountBuildState(context.Background(), a)
	if e != nil || v.State != "CANNOT_CREATE" || v.ProviderCanCreate || v.Buildable == nil || *v.Buildable != 0 {
		t.Fatal(v, e)
	}
	sqlMust(t, db, `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Account,StatusMessage}','"Billing restriction"',true)`)
	v, e = s.accountBuildState(context.Background(), a)
	if e != nil || v.State != "PROVIDER_WARNING" || v.ProviderCanCreate {
		t.Fatal(v, e)
	}
}

func TestAccountPurgeQueueRetainsNativeEvidenceAndRejectsDrift(t *testing.T) {
	for _, change := range []string{"native", "provider", "resources", "request_time"} {
		t.Run(change, func(t *testing.T) {
			db := adminTestDB(t)
			a, d, _ := seedPurgeDependencies(t, db)
			ctx := context.Background()
			if e := app.QueueSavedAccountPurge(ctx, db, a, "LOCKED", 1, purgeConfirmation(t, db, a)); e != nil {
				t.Fatal(e)
			}
			switch change {
			case "native":
				sqlMust(t, db, "INSERT INTO worker_recovery_checkpoints(kind,item_id,account_id) SELECT 'deployment',id::text,account_id FROM deployments WHERE droplet_id=$1 AND account_id=$2 LIMIT 1", d, a)
			case "provider":
				sqlMust(t, db, "UPDATE accounts SET provider_state='ACTIVE' WHERE id=$1", a)
			case "resources":
				sqlMust(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES(gen_random_uuid(),$1,'new-unacknowledged','RETIRING')", a)
			case "request_time":
				sqlMust(t, db, "UPDATE accounts SET deletion_requested_at=now()-interval '4 minutes' WHERE id=$1", a)
			}
			handled, e := (app.Container{DB: db}).ProcessSavedAccountPurge(ctx)
			if !handled || e == nil {
				t.Fatal("unsafe request executed", handled, e)
			}
			var n int
			var state string
			if e = db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", a).Scan(&n); e != nil || n != 1 {
				t.Fatal("account evidence erased", n, e)
			}
			if e = db.QueryRow("SELECT state FROM account_purge_requests WHERE account_id=$1", a).Scan(&state); e != nil {
				t.Fatal(e)
			}
			if change == "native" {
				if state != "RETRY" {
					t.Fatal(state)
				}
				// Only simulate the native owner's completion within this isolated fixture.
				sqlMust(t, db, "DELETE FROM worker_recovery_checkpoints WHERE account_id=$1", a)
				sqlMust(t, db, "UPDATE account_purge_requests SET next_attempt_at=now() WHERE account_id=$1", a)
				if handled, e = (app.Container{DB: db}).ProcessSavedAccountPurge(ctx); !handled || e != nil {
					t.Fatal(handled, e)
				}
			} else if state != "NEEDS_REVIEW" {
				t.Fatal(state)
			}
		})
	}
}

func TestAccountPurgeQueueRejectsUnrequestedAndBadScope(t *testing.T) {
	db := adminTestDB(t)
	a, _, _ := seedPanel(t, db, "http://keep.invalid")
	for _, id := range []string{a, "../bad"} {
		if e := app.QueueSavedAccountPurge(context.Background(), db, id, "LOCKED", 1, purgeConfirmation(t, db, id)); !errors.Is(e, app.ErrAccountPurgeConflict) {
			t.Fatal(e)
		}
	}
	if withinObservationSchedule(time.Now().Add(time.Minute), time.Now(), 300) {
		t.Fatal("future evidence accepted")
	}
}

func TestAccountPurgeQueueSkipsLockedPrefix(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	conn, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	defer conn.ExecContext(ctx, "SELECT pg_advisory_unlock_all()")
	var last string
	for i := 0; i < 34; i++ {
		id := perfUUID()
		last = id
		sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref,enabled,provider_state,runtime_status,deletion_requested_at) VALUES($1,'queue fixture','vultr','fixture',false,'LOCKED','DELETE_PENDING',now()-interval '4 minutes')", id)
		if e = app.QueueSavedAccountPurge(ctx, db, id, "LOCKED", 0, purgeConfirmation(t, db, id)); e != nil {
			t.Fatal(e)
		}
		sqlMust(t, db, "UPDATE account_purge_requests SET next_attempt_at=now()-interval '1 minute'+($2*interval '1 millisecond') WHERE account_id=$1", id, i)
		if i < 33 {
			if _, e = conn.ExecContext(ctx, "SELECT pg_advisory_lock(hashtextextended($1,714))", id); e != nil {
				t.Fatal(e)
			}
		}
	}
	bounded, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if handled, e := (app.Container{DB: db}).ProcessSavedAccountPurge(bounded); !handled || e != nil {
		t.Fatal("locked prefix starvation", handled, e)
	}
	var n int
	if e = db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", last).Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	if e = db.QueryRow("SELECT count(*) FROM accounts").Scan(&n); e != nil || n != 33 {
		t.Fatal("wrong scope", n, e)
	}
}

func TestAccountPurgeConsentBindsDeletionAndSameCountScope(t *testing.T) {
	for _, change := range []string{"request_before_queue", "scope_before_queue", "scope_before_fast_path", "scope_after_queue"} {
		t.Run(change, func(t *testing.T) {
			db := adminTestDB(t)
			a, d, _ := seedPurgeDependencies(t, db)
			consent := purgeConfirmation(t, db, a)
			ctx := context.Background()
			if change == "scope_after_queue" {
				if err := app.QueueSavedAccountPurge(ctx, db, a, "LOCKED", 1, consent); err != nil {
					t.Fatal(err)
				}
			}
			if change == "request_before_queue" {
				sqlMust(t, db, "UPDATE accounts SET deletion_requested_at=deletion_requested_at-interval '1 minute' WHERE id=$1", a)
			} else {
				sqlMust(t, db, "UPDATE droplets SET provider_resource_id='same-count-replacement' WHERE id=$1", d)
				sqlMust(t, db, "UPDATE resources SET provider_resource_id='same-count-replacement' WHERE account_id=$1", a)
			}
			progress := (&Server{DB: db}).accountDeletionProgress(ctx, a).(map[string]any)
			if progress["remaining_servers"] != 1 {
				t.Fatal("fixture must keep count identical", progress)
			}
			var err error
			switch change {
			case "scope_before_fast_path":
				err = app.PurgeSavedAccount(ctx, db, a, "LOCKED", 1, consent)
			case "scope_after_queue":
				_, err = (app.Container{DB: db}).ProcessSavedAccountPurge(ctx)
				var state string
				if e := db.QueryRow("SELECT state FROM account_purge_requests WHERE account_id=$1", a).Scan(&state); e != nil || state != "NEEDS_REVIEW" {
					t.Fatal(state, e)
				}
			default:
				err = app.QueueSavedAccountPurge(ctx, db, a, "LOCKED", 1, consent)
			}
			if !errors.Is(err, app.ErrAccountPurgeConflict) {
				t.Fatal("stale consent accepted", err)
			}
			var n int
			if err = db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", a).Scan(&n); err != nil || n != 1 {
				t.Fatal("account removed", n, err)
			}
		})
	}
}

func TestAccountPurgeConsentRejectsDiscoveredNativeIdentities(t *testing.T) {
	for _, kind := range []string{"operation", "deployment", "deployment_key", "ssh_key", "unmanaged_resource"} {
		t.Run(kind, func(t *testing.T) {
			db := adminTestDB(t)
			a, _, _ := seedPurgeDependencies(t, db)
			ctx := context.Background()
			sqlMust(t, db, "INSERT INTO operations(id,account_id,kind,idempotency_key,state) VALUES(gen_random_uuid(),$1,'CREATE_DROPLET','unknown-discovery','unknown')", a)
			consent := purgeConfirmation(t, db, a)
			if err := app.QueueSavedAccountPurge(ctx, db, a, "LOCKED", 1, consent); err != nil {
				t.Fatal(err)
			}
			// Native ownership is released only after its new evidence is persisted.
			sqlMust(t, db, "INSERT INTO worker_recovery_checkpoints(kind,item_id,account_id) SELECT 'deployment',id::text,account_id FROM deployments WHERE account_id=$1 LIMIT 1", a)
			switch kind {
			case "operation":
				sqlMust(t, db, "UPDATE operations SET resource_id='newly-discovered',state='failed' WHERE account_id=$1", a)
			case "deployment":
				sqlMust(t, db, "UPDATE deployments SET provider_id='newly-discovered',state='FAILED' WHERE account_id=$1", a)
			case "deployment_key":
				sqlMust(t, db, "UPDATE deployments SET profile_snapshot=jsonb_set(COALESCE(profile_snapshot,'{}'),'{ssh_provider_key_id}','\"new-key\"') WHERE account_id=$1", a)
			case "ssh_key":
				sqlMust(t, db, "INSERT INTO account_deletion_keys(account_id,provider_key_id) VALUES($1,'new-key')", a)
			case "unmanaged_resource":
				sqlMust(t, db, "INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed) VALUES(gen_random_uuid(),$1,'digitalocean','newly-discovered','server','ready',false)", a)
			}
			sqlMust(t, db, "DELETE FROM worker_recovery_checkpoints WHERE account_id=$1", a)
			progress := (&Server{DB: db}).accountDeletionProgress(ctx, a).(map[string]any)
			if progress["remaining_servers"] != 1 {
				t.Fatal("count changed", progress)
			}
			if err := app.PurgeSavedAccount(ctx, db, a, "LOCKED", 1, consent); !errors.Is(err, app.ErrAccountPurgeConflict) {
				t.Fatal("fast path erased newly discovered evidence", err)
			}
			if _, err := (app.Container{DB: db}).ProcessSavedAccountPurge(ctx); !errors.Is(err, app.ErrAccountPurgeConflict) {
				t.Fatal("queue erased newly discovered evidence", err)
			}
			var state string
			if err := db.QueryRow("SELECT state FROM account_purge_requests WHERE account_id=$1", a).Scan(&state); err != nil || state != "NEEDS_REVIEW" {
				t.Fatal(state, err)
			}
			var n int
			if err := db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", a).Scan(&n); err != nil || n != 1 {
				t.Fatal(n, err)
			}
		})
	}
}

func TestAccountPurgeMigrationRollbackPreservesOutstandingIntent(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/000169_account_cleanup_health.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"QUEUED", "RETRY", "NEEDS_REVIEW"} {
		t.Run(state, func(t *testing.T) {
			db := adminTestDB(t)
			a, _, _ := seedPurgeDependencies(t, db)
			if err := app.QueueSavedAccountPurge(context.Background(), db, a, "LOCKED", 1, purgeConfirmation(t, db, a)); err != nil {
				t.Fatal(err)
			}
			sqlMust(t, db, "UPDATE account_purge_requests SET state=$2 WHERE account_id=$1", a, state)
			if _, err := db.Exec(string(raw)); err == nil || !strings.Contains(err.Error(), "durable intent") {
				t.Fatal("unsafe schema rollback accepted", err)
			}
			var got string
			if err := db.QueryRow("SELECT state FROM account_purge_requests WHERE account_id=$1", a).Scan(&got); err != nil || got != state {
				t.Fatal("intent lost", got, err)
			}
			var cadence int
			if err := db.QueryRow("SELECT provider_inventory_interval_seconds FROM accounts WHERE id=$1", a).Scan(&cadence); err != nil {
				t.Fatal("partial schema rollback", err)
			}
			// Explicit resolution in the isolated fixture allows a genuinely empty rollback.
			sqlMust(t, db, "DELETE FROM account_purge_requests WHERE account_id=$1", a)
			if _, err := db.Exec(string(raw)); err != nil {
				t.Fatal("empty rollback failed", err)
			}
			var gone bool
			if err := db.QueryRow("SELECT to_regclass('account_purge_requests') IS NULL").Scan(&gone); err != nil || !gone {
				t.Fatal(gone, err)
			}
		})
	}
}
