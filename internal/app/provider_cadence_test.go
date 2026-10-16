package app

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"testing"
	"time"
)

func TestProviderCadenceCannotChangeBeforeItsObservation(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	rt := observationFixture(t, db)
	ctx := context.Background()
	execBootstrap(t, db, "UPDATE accounts SET provider_inventory_interval_seconds=300,provider_checked_at=now()-interval '200 seconds' WHERE id=$1", rt.Config.ID)
	execBootstrap(t, db, "INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical,created_at) VALUES(gen_random_uuid(),$1,'digitalocean',2,'{}','{}',now()-interval '200 seconds')", rt.Config.ID)
	var before time.Time
	if e := db.QueryRow("SELECT provider_checked_at FROM accounts WHERE id=$1", rt.Config.ID).Scan(&before); e != nil {
		t.Fatal(e)
	}
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if _, e = tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "provider-refresh:"+rt.Config.ID); e != nil {
		t.Fatal(e)
	}
	// A sibling owns the refresh. Planning a faster cadence must not relabel
	// the still-valid previous observation while no new evidence exists.
	c.RefreshProviderSnapshots(ctx, time.Minute)
	var cadence int
	var after time.Time
	if e = db.QueryRow("SELECT provider_inventory_interval_seconds,provider_checked_at FROM accounts WHERE id=$1", rt.Config.ID).Scan(&cadence, &after); e != nil {
		t.Fatal(e)
	}
	if cadence != 300 || !before.Equal(after) {
		t.Fatalf("unpublished observation changed cadence/time: %d %v", cadence, after)
	}
}

func TestProviderCadencePublicationIsAtomicAndFenced(t *testing.T) {
	for _, scenario := range []string{"success", "error", "rollback", "obsolete", "credential_replaced", "invalid", "unscheduled"} {
		t.Run(scenario, func(t *testing.T) {
			db := installerBootstrapDB(t)
			c := Container{DB: db}
			rt := observationFixture(t, db)
			ctx := context.Background()
			execBootstrap(t, db, "UPDATE accounts SET provider_inventory_interval_seconds=300,provider_checked_at=now()-interval '200 seconds' WHERE id=$1", rt.Config.ID)
			var before time.Time
			if e := db.QueryRow("SELECT provider_checked_at FROM accounts WHERE id=$1", rt.Config.ID).Scan(&before); e != nil {
				t.Fatal(e)
			}
			attempt, e := c.BeginProviderAttempt(ctx, rt.Config.ID, true)
			if e != nil {
				t.Fatal(e)
			}
			attempt.InventoryIntervalSeconds = 60
			obs := providers.Observation{Account: providers.Account{Status: "active"}, Capacity: providers.Capacity{LimitKnown: true, ComputeLimit: 10}}
			if scenario == "rollback" {
				execBootstrap(t, db, "CREATE FUNCTION reject_cadence_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture snapshot failure'; END $$; CREATE TRIGGER reject_cadence_snapshot BEFORE INSERT ON provider_snapshots FOR EACH ROW EXECUTE FUNCTION reject_cadence_snapshot()")
			}
			if scenario == "obsolete" {
				fresh, e := c.BeginProviderAttempt(ctx, rt.Config.ID, true)
				if e != nil {
					t.Fatal(e)
				}
				fresh.InventoryIntervalSeconds = 300
				if ok, e := c.PublishProviderObservation(ctx, rt, fresh, obs, "READY", "fresh"); !ok || e != nil {
					t.Fatal(ok, e)
				}
				if e := db.QueryRow("SELECT provider_checked_at FROM accounts WHERE id=$1", rt.Config.ID).Scan(&before); e != nil {
					t.Fatal(e)
				}
			}
			if scenario == "credential_replaced" {
				execBootstrap(t, db, "UPDATE accounts SET secret_ref='replacement' WHERE id=$1", rt.Config.ID)
			}
			if scenario == "invalid" {
				attempt.InventoryIntervalSeconds = 301
			}
			if scenario == "unscheduled" {
				attempt.InventoryIntervalSeconds = 0
			}
			var ok bool
			if scenario == "error" {
				ok, e = c.RecordProviderObservation(ctx, attempt, ProviderStateTransportError, errors.New("offline"), "network failure")
			} else {
				ok, e = c.PublishProviderObservation(ctx, rt, attempt, obs, "READY", "test")
			}
			wantOK := scenario == "success" || scenario == "error" || scenario == "unscheduled"
			wantErr := scenario == "rollback" || scenario == "invalid"
			if ok != wantOK || (e != nil) != wantErr {
				t.Fatal(scenario, ok, e)
			}
			var cadence int
			var after time.Time
			var snapshots int
			if e := db.QueryRow("SELECT provider_inventory_interval_seconds,provider_checked_at,(SELECT count(*) FROM provider_snapshots WHERE account_id=$1) FROM accounts WHERE id=$1", rt.Config.ID).Scan(&cadence, &after, &snapshots); e != nil {
				t.Fatal(e)
			}
			wantCadence := 300
			if scenario == "success" || scenario == "error" {
				wantCadence = 60
			}
			if cadence != wantCadence {
				t.Fatalf("cadence %d want %d", cadence, wantCadence)
			}
			if scenario == "success" || scenario == "unscheduled" {
				if !after.After(before) || snapshots != 1 {
					t.Fatal("no atomic fresh observation", after, snapshots)
				}
			} else if !after.Equal(before) {
				t.Fatal("failure/rejected attempt forged observation time")
			}
		})
	}
}

func TestProviderCadenceOlderRestrictionPreservesNewerSchedule(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	rt := observationFixture(t, db)
	ctx := context.Background()
	old, e := c.BeginProviderAttempt(ctx, rt.Config.ID, true)
	if e != nil {
		t.Fatal(e)
	}
	old.InventoryIntervalSeconds = 60
	fresh := observationAttempt(t, c, rt)
	fresh.InventoryIntervalSeconds = 300
	if ok, e := c.PublishProviderObservation(ctx, rt, fresh, providers.Observation{Account: providers.Account{Status: "unknown"}}, "PROVIDER_BLOCKED", "unknown status"); !ok || e != nil {
		t.Fatal(ok, e)
	}
	// Independent authoritative restriction remains valid, but is not evidence
	// that the newer complete observation used the old coordinator schedule.
	if ok, e := c.RecordProviderObservation(ctx, old, ProviderStateLocked, &providers.Error{Class: providers.ErrorAccountLocked, StatusCode: 403}, "locked"); !ok || e != nil {
		t.Fatal(ok, e)
	}
	var cadence int
	var state string
	if e := db.QueryRow("SELECT provider_inventory_interval_seconds,provider_state FROM accounts WHERE id=$1", rt.Config.ID).Scan(&cadence, &state); e != nil {
		t.Fatal(e)
	}
	if cadence != 300 || state != "LOCKED" {
		t.Fatal("late restriction lost independent evidence fence", cadence, state)
	}
}
