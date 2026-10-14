package app

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"testing"
	"time"
)

func observationFixture(t *testing.T, db *sql.DB) AccountRuntime {
	t.Helper()
	var id string
	if err := db.QueryRow(`INSERT INTO accounts(id,name,provider,secret_ref,enabled,provider_state,runtime_status) VALUES(gen_random_uuid(),'observation fixture','digitalocean','fixture',true,'ACTIVE','READY') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, `INSERT INTO network_profiles(id,account_id,mode) VALUES(gen_random_uuid(),$1,'direct')`, id)
	return AccountRuntime{Config: AccountConfig{ID: id, Provider: "digitalocean", SecretRef: "fixture", Network: network.Profile{Mode: "direct"}}}
}
func TestProviderObservationAtomicPublicationAndRecovery(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	rt := observationFixture(t, db)
	ctx := context.Background()
	obs := providers.Observation{Account: providers.Account{ID: "fixture", Status: "locked", StatusMessage: "Support required"}, Capacity: providers.Capacity{LimitKnown: true, ComputeLimit: 3}, ObservedAt: time.Now().UTC()}
	ok, err := c.PublishProviderObservation(ctx, rt, observationAttempt(t, c, rt), obs, "READY", "read succeeded")
	if !ok || err != nil {
		t.Fatal(ok, err)
	}
	var state, runtime string
	var n int
	db.QueryRow(`SELECT provider_state,runtime_status FROM accounts WHERE id=$1`, rt.Config.ID).Scan(&state, &runtime)
	if state != "LOCKED" || runtime != "PROVIDER_LOCKED" {
		t.Fatal(state, runtime)
	}
	c.RecordProviderObservation(ctx, observationAttempt(t, c, rt), ProviderStateProxyError, ErrNetworkNotReady, `{"provider_error":"account network not ready"}`)
	db.QueryRow(`SELECT provider_state,COALESCE(provider_error_state,'') FROM accounts WHERE id=$1`, rt.Config.ID).Scan(&state, &runtime)
	if state != "LOCKED" || runtime != "PROXY_ERROR" {
		t.Fatal("network error hid lock", state, runtime)
	}
	obs.Account.Status = "active"
	ok, err = c.PublishProviderObservation(ctx, rt, ProviderAttempt{AccountID: rt.Config.ID, SecretRef: rt.Config.SecretRef, Started: time.Now().Add(-time.Hour)}, obs, "READY", "delayed success")
	if ok || err != nil {
		t.Fatal("stale success accepted", ok, err)
	}
	db.QueryRow(`SELECT count(*) FROM provider_snapshots WHERE account_id=$1`, rt.Config.ID).Scan(&n)
	if n != 1 {
		t.Fatal("obsolete snapshot published", n)
	}
	ok, err = c.PublishProviderObservation(ctx, rt, observationAttempt(t, c, rt), obs, "READY", "new authoritative success")
	if !ok || err != nil {
		t.Fatal(ok, err)
	}
	db.QueryRow(`SELECT provider_state,COALESCE(provider_error_state,'') FROM accounts WHERE id=$1`, rt.Config.ID).Scan(&state, &runtime)
	if state != "ACTIVE" || runtime != "" {
		t.Fatal(state, runtime)
	}
	for _, change := range []string{"secret_ref='replacement'", "enabled=false", "deletion_requested_at=now()"} {
		other := observationFixture(t, db)
		execBootstrap(t, db, "UPDATE accounts SET "+change+" WHERE id=$1", other.Config.ID)
		ok, err = c.PublishProviderObservation(ctx, other, ProviderAttempt{AccountID: other.Config.ID, SecretRef: other.Config.SecretRef, Started: time.Now()}, obs, "READY", "stale identity")
		if ok || err != nil {
			t.Fatal(change, ok, err)
		}
	}
	// A failed snapshot insert rolls back the readiness update in the same transaction.
	other := observationFixture(t, db)
	execBootstrap(t, db, `UPDATE accounts SET provider_state='LOCKED',runtime_status='PROVIDER_LOCKED' WHERE id=$1`, other.Config.ID)
	execBootstrap(t, db, `CREATE FUNCTION reject_observation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture snapshot failure'; END $$; CREATE TRIGGER reject_observation BEFORE INSERT ON provider_snapshots FOR EACH ROW EXECUTE FUNCTION reject_observation()`)
	ok, err = c.PublishProviderObservation(ctx, other, ProviderAttempt{AccountID: other.Config.ID, SecretRef: other.Config.SecretRef, Started: time.Now()}, obs, "READY", "must rollback")
	if ok || err == nil {
		t.Fatal("write failure accepted", ok, err)
	}
	db.QueryRow(`SELECT provider_state,runtime_status FROM accounts WHERE id=$1`, other.Config.ID).Scan(&state, &runtime)
	if state != "LOCKED" || runtime != "PROVIDER_LOCKED" {
		t.Fatal("state escaped rollback", state, runtime)
	}
}

func TestProviderObservationCannotOverwriteConcurrentRestriction(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	rt := observationFixture(t, db)
	ctx := context.Background()
	started := observationAttempt(t, c, rt)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, rt.Config.ID); err != nil {
		t.Fatal(err)
	}
	type result struct {
		ok  bool
		err error
	}
	done := make(chan result, 1)
	go func() {
		ok, e := c.PublishProviderObservation(ctx, rt, started, providers.Observation{Account: providers.Account{Status: "active"}}, "READY", "old success")
		done <- result{ok, e}
	}()
	select {
	case x := <-done:
		t.Fatal("publication bypassed account lock", x)
	case <-time.After(30 * time.Millisecond):
	}
	if _, err = tx.Exec(`UPDATE accounts SET provider_state='LOCKED',runtime_status='PROVIDER_LOCKED',provider_state_at=clock_timestamp(),provider_checked_at=clock_timestamp() WHERE id=$1`, rt.Config.ID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case x := <-done:
		if x.ok || x.err != nil {
			t.Fatal(x)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("publication did not release")
	}
	var state string
	var n int
	db.QueryRow(`SELECT provider_state FROM accounts WHERE id=$1`, rt.Config.ID).Scan(&state)
	db.QueryRow(`SELECT count(*) FROM provider_snapshots WHERE account_id=$1`, rt.Config.ID).Scan(&n)
	if state != "LOCKED" || n != 0 {
		t.Fatal(state, n)
	}
}

func observationAttempt(t *testing.T, c Container, rt AccountRuntime) ProviderAttempt {
	t.Helper()
	a, err := c.BeginProviderAttempt(context.Background(), rt.Config.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestProviderObservationWritersPreserveIndependentEvidence(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	rt := observationFixture(t, db)
	ctx := context.Background()
	lock := &providers.Error{Class: providers.ErrorAccountLocked, StatusCode: 200, Code: "ACCOUNT_LOCKED", Message: "Team locked"}
	early := observationAttempt(t, c, rt)
	late := observationAttempt(t, c, rt)
	if ok, e := c.RecordProviderObservation(ctx, late, ProviderStateProxyError, ErrNetworkNotReady, "local failure"); !ok || e != nil {
		t.Fatal(ok, e)
	}
	if ok, e := c.RecordProviderObservation(ctx, early, ProviderStateLocked, lock, "account lock"); !ok || e != nil {
		t.Fatal(ok, e)
	}
	var state, observation, accountStatus string
	var before sql.NullTime
	db.QueryRow(`SELECT provider_state,provider_error_state,provider_account_status,provider_refresh_attempt_at FROM accounts WHERE id=$1`, rt.Config.ID).Scan(&state, &observation, &accountStatus, &before)
	if state != "LOCKED" || observation != "PROXY_ERROR" || accountStatus != "locked" {
		t.Fatal(state, observation, accountStatus)
	}
	a := observationAttempt(t, c, rt)
	if ok, e := c.RecordProviderObservation(ctx, a, ProviderStateRateLimited, &providers.Error{Class: providers.ErrorRateLimited, StatusCode: 429}, "rate limited"); !ok || e != nil {
		t.Fatal(ok, e)
	}
	db.QueryRow(`SELECT provider_state,provider_error_state FROM accounts WHERE id=$1`, rt.Config.ID).Scan(&state, &observation)
	if state != "LOCKED" || observation != "RATE_LIMITED" {
		t.Fatal(state, observation)
	}
	old := observationAttempt(t, c, rt)
	fresh := observationAttempt(t, c, rt)
	if ok, e := c.PublishProviderObservation(ctx, rt, fresh, providers.Observation{Account: providers.Account{Status: "active"}}, "READY", "fresh"); !ok || e != nil {
		t.Fatal(ok, e)
	}
	for _, s := range []string{ProviderStateLocked, ProviderStateProxyError} {
		if ok, e := c.RecordProviderObservation(ctx, old, s, lock, "late result"); ok || e != nil {
			t.Fatal("late writer", s, ok, e)
		}
	}
	scheduled, e := c.BeginProviderAttempt(ctx, rt.Config.ID, true)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.RecordProviderObservation(ctx, observationAttempt(t, c, rt), ProviderStateProxyError, ErrNetworkNotReady, "local"); e != nil {
		t.Fatal(e)
	}
	var attemptAt time.Time
	db.QueryRow(`SELECT provider_refresh_attempt_at FROM accounts WHERE id=$1`, rt.Config.ID).Scan(&attemptAt)
	if !attemptAt.Equal(scheduled.Started) {
		t.Fatal("local error moved scheduled provider deadline")
	}
}

func TestProviderObservationSameReferenceCredentialReplacement(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	rt := observationFixture(t, db)
	ctx := context.Background()
	execBootstrap(t, db, `INSERT INTO secrets(id,account_id,kind,ciphertext,nonce,key_version) VALUES('fixture',$1,'fixture','x','x',1)`, rt.Config.ID)
	old := observationAttempt(t, c, rt)
	execBootstrap(t, db, `UPDATE secrets SET updated_at=clock_timestamp(),ciphertext='replacement' WHERE account_id=$1`, rt.Config.ID)
	if ok, e := c.RecordProviderObservation(ctx, old, ProviderStateLocked, &providers.Error{Class: providers.ErrorAccountLocked}, "old credential"); ok || e != nil {
		t.Fatal(ok, e)
	}
	if ok, e := c.PublishProviderObservation(ctx, rt, old, providers.Observation{Account: providers.Account{Status: "active"}}, "READY", "old credential"); ok || e != nil {
		t.Fatal(ok, e)
	}
	fresh := observationAttempt(t, c, rt)
	if ok, e := c.PublishProviderObservation(ctx, rt, fresh, providers.Observation{Account: providers.Account{Status: "active"}}, "READY", "new credential"); !ok || e != nil {
		t.Fatal(ok, e)
	}
}

func TestProviderObservationUnknownStatusCannotResolveRestriction(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	rt := observationFixture(t, db)
	ctx := context.Background()
	execBootstrap(t, db, `UPDATE accounts SET provider_state='LOCKED' WHERE id=$1`, rt.Config.ID)
	for _, status := range []string{"unknown", "warning", "trial_restricted", ""} {
		if ok, e := c.PublishProviderObservation(ctx, rt, observationAttempt(t, c, rt), providers.Observation{Account: providers.Account{Status: status}}, "PROVIDER_BLOCKED", "restricted"); !ok || e != nil {
			t.Fatal(ok, e)
		}
		var state, runtime string
		db.QueryRow(`SELECT provider_state,runtime_status FROM accounts WHERE id=$1`, rt.Config.ID).Scan(&state, &runtime)
		if state != "LOCKED" || runtime != "PROVIDER_LOCKED" {
			t.Fatal(status, state, runtime)
		}
	}
}

func TestProviderObservationObsoleteNetworkCannotClearError(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	rt := observationFixture(t, db)
	ctx := context.Background()
	var proxy string
	if e := db.QueryRow(`INSERT INTO proxies(id,name,type,host,port,status,last_success_at) VALUES(gen_random_uuid(),'observation proxy','http','127.0.0.1',18080,'healthy',now()) RETURNING id::text`).Scan(&proxy); e != nil {
		t.Fatal(e)
	}
	execBootstrap(t, db, `UPDATE network_profiles SET mode='proxy_required',proxy_id=$2 WHERE account_id=$1`, rt.Config.ID, proxy)
	execBootstrap(t, db, `INSERT INTO account_network_identities(account_id,timezone,locale,exit_ip,subnet_key,last_health_ok,last_health_at) VALUES($1,'UTC','en-US','203.0.113.211','203.0.113.0/24',true,now())`, rt.Config.ID)
	execBootstrap(t, db, `INSERT INTO proxy_runtime_state(account_id,proxy_id,provider,generation,health_state,circuit_state) VALUES($1,$2,'digitalocean',3,'healthy','closed')`, rt.Config.ID, proxy)
	rt.Config.Network.Mode = "proxy_required"
	rt.Config.Proxy = &network.Proxy{ID: proxy}
	rt.TransportGeneration = 3
	if e := db.QueryRow(`SELECT transport_epoch FROM account_transport_state WHERE account_id=$1`, rt.Config.ID).Scan(&rt.TransportEpoch); e != nil {
		t.Fatal(e)
	}
	for _, change := range []string{`UPDATE proxy_runtime_state SET generation=generation+1 WHERE account_id=$1`, `UPDATE account_transport_state SET transport_epoch=transport_epoch+1 WHERE account_id=$1`, `UPDATE account_network_identities SET exit_ip='203.0.113.212',sticky_session='replacement-session' WHERE account_id=$1`, `UPDATE account_network_identities SET last_health_ok=false WHERE account_id=$1`} {
		if _, e := c.RecordProviderObservation(ctx, observationAttempt(t, c, rt), ProviderStateProxyError, ErrNetworkNotReady, "network"); e != nil {
			t.Fatal(e)
		}
		a := observationAttempt(t, c, rt)
		execBootstrap(t, db, change, rt.Config.ID)
		if ok, e := c.PublishProviderObservation(ctx, rt, a, providers.Observation{Account: providers.Account{Status: "active"}}, "READY", "read completed earlier"); !ok || e != nil {
			t.Fatal(ok, e)
		}
		var runtime, observed string
		db.QueryRow(`SELECT runtime_status,provider_error_state FROM accounts WHERE id=$1`, rt.Config.ID).Scan(&runtime, &observed)
		if runtime == "READY" || observed != "PROXY_ERROR" {
			t.Fatal(runtime, observed)
		}
		db.QueryRow(`SELECT generation FROM proxy_runtime_state WHERE account_id=$1`, rt.Config.ID).Scan(&rt.TransportGeneration)
		db.QueryRow(`SELECT transport_epoch FROM account_transport_state WHERE account_id=$1`, rt.Config.ID).Scan(&rt.TransportEpoch)
	}
	execBootstrap(t, db, `UPDATE account_network_identities SET last_health_ok=true,last_health_at=clock_timestamp() WHERE account_id=$1`, rt.Config.ID)
	if ok, e := c.PublishProviderObservation(ctx, rt, observationAttempt(t, c, rt), providers.Observation{Account: providers.Account{Status: "active"}}, "READY", "same identity recovered"); !ok || e != nil {
		t.Fatal(ok, e)
	}
	var finalStatus string
	db.QueryRow(`SELECT runtime_status FROM accounts WHERE id=$1`, rt.Config.ID).Scan(&finalStatus)
	if finalStatus != "READY" {
		t.Fatal("native network recovery did not release observation error", finalStatus)
	}
}

func TestProviderObservationRetainsLegacyLockAcrossUnknownReads(t *testing.T) {
	for _, status := range []string{"unknown", "warning", "trial_restricted", ""} {
		t.Run(status, func(t *testing.T) {
			db := installerBootstrapDB(t)
			c := Container{DB: db}
			rt := observationFixture(t, db)
			ctx := context.Background()
			execBootstrap(t, db, `INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical) VALUES(gen_random_uuid(),$1,'digitalocean',2,'{}','{"Account":{"Status":"locked"}}')`, rt.Config.ID)
			a := observationAttempt(t, c, rt)
			if _, e := c.RecordProviderObservation(ctx, observationAttempt(t, c, rt), ProviderStateProxyError, ErrNetworkNotReady, "newer independent network failure"); e != nil {
				t.Fatal(e)
			}
			if ok, e := c.PublishProviderObservation(ctx, rt, a, providers.Observation{Account: providers.Account{Status: status}}, "PROVIDER_BLOCKED", "unknown status"); !ok || e != nil {
				t.Fatal(ok, e)
			}
			var state, runtime, networkError string
			db.QueryRow(`SELECT provider_state,runtime_status,COALESCE(provider_error_state,'') FROM accounts WHERE id=$1`, rt.Config.ID).Scan(&state, &runtime, &networkError)
			if state != "LOCKED" || runtime != "PROVIDER_LOCKED" || networkError != "PROXY_ERROR" {
				t.Fatal(status, state, runtime, networkError)
			}
			if ok, e := c.PublishProviderObservation(ctx, rt, observationAttempt(t, c, rt), providers.Observation{Account: providers.Account{Status: "active"}}, "READY", "authoritative resolution"); !ok || e != nil {
				t.Fatal(ok, e)
			}
			db.QueryRow(`SELECT provider_state,COALESCE(provider_error_state,'') FROM accounts WHERE id=$1`, rt.Config.ID).Scan(&state, &networkError)
			if state != "ACTIVE" || networkError != "" {
				t.Fatal(state, networkError)
			}
		})
	}
}

func TestProviderObservationNonResolvingReadCannotSuppressDelayedLock(t *testing.T) {
	for _, status := range []string{"unknown", "warning", "trial_restricted", ""} {
		for _, lockFirst := range []bool{false, true} {
			for _, completeLock := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/lock-first=%t/complete-lock=%t", status, lockFirst, completeLock), func(t *testing.T) {
					db := installerBootstrapDB(t)
					c := Container{DB: db}
					rt := observationFixture(t, db)
					ctx := context.Background()
					lockAttempt := observationAttempt(t, c, rt)
					completeAttempt := observationAttempt(t, c, rt)
					lockErr := &providers.Error{Class: providers.ErrorAccountLocked, StatusCode: 422, Message: "account is locked"}
					if _, e := c.RecordProviderObservation(ctx, observationAttempt(t, c, rt), ProviderStateProxyError, ErrNetworkNotReady, "newer network failure"); e != nil {
						t.Fatal(e)
					}
					publishLock := func() (bool, error) {
						if completeLock {
							return c.PublishProviderObservation(ctx, rt, lockAttempt, providers.Observation{Account: providers.Account{Status: "locked"}}, "READY", "complete lock")
						}
						return c.RecordProviderObservation(ctx, lockAttempt, ProviderStateLocked, lockErr, "authoritative lock")
					}
					if lockFirst {
						if ok, e := publishLock(); !ok || e != nil {
							t.Fatal("lock suppressed", ok, e)
						}
					}
					if ok, e := c.PublishProviderObservation(ctx, rt, completeAttempt, providers.Observation{Account: providers.Account{Status: status}}, "READY", "non-resolving read"); !ok || e != nil {
						t.Fatal(ok, e)
					}
					if !lockFirst {
						if ok, e := publishLock(); !ok || e != nil {
							t.Fatal("delayed lock suppressed", ok, e)
						}
					}
					var state, observation, runtime, lastStatus string
					var watermark time.Time
					var snapshots int
					db.QueryRow(`SELECT provider_state,COALESCE(provider_error_state,''),runtime_status,provider_observation_started_at FROM accounts WHERE id=$1`, rt.Config.ID).Scan(&state, &observation, &runtime, &watermark)
					db.QueryRow(`SELECT canonical->'Account'->>'Status' FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, rt.Config.ID).Scan(&lastStatus)
					db.QueryRow(`SELECT count(*) FROM provider_snapshots WHERE account_id=$1`, rt.Config.ID).Scan(&snapshots)
					wantSnapshots := 1
					if completeLock && lockFirst {
						wantSnapshots = 2
					}
					if state != "LOCKED" || observation != "PROXY_ERROR" || runtime != "PROVIDER_LOCKED" || !watermark.Equal(completeAttempt.Started) || lastStatus != status || snapshots != wantSnapshots {
						t.Fatal(state, observation, runtime, watermark, lastStatus, snapshots)
					}
					if ok, e := c.PublishProviderObservation(ctx, rt, observationAttempt(t, c, rt), providers.Observation{Account: providers.Account{Status: "active"}}, "READY", "resolved"); !ok || e != nil {
						t.Fatal(ok, e)
					}
					if ok, e := publishLock(); ok || e != nil {
						t.Fatal("old lock resurrected", ok, e)
					}
				})
			}
		}
	}
}
