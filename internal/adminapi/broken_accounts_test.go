package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"strings"
	"testing"
)

func TestAccountHealthDetailsMatchSevenAccountCounts(t *testing.T) {
	db := adminTestDB(t)
	for _, name := range []string{"S1", "Sor oc", "Digi oc", "Oceano new1", "Linode", "Vultr1", "Vultr2"} {
		provider := "digitalocean"
		if name == "Digi oc" {
			provider = "upcloud"
		} // Also exercise a saved create restriction beside the lock.
		id := seedBuildAccount(t, db, provider)
		sqlMust(t, db, "UPDATE accounts SET name=$2 WHERE id=$1", id, name)
		putBuildSnapshot(t, db, id, "active", true, 10, 5)
		if name == "S1" || name == "Sor oc" || name == "Digi oc" {
			sqlMust(t, db, `UPDATE accounts SET provider_state='LOCKED',runtime_status='PROVIDER_LOCKED',provider_state_detail='{"provider_error":"Explicit provider account lock"}' WHERE id=$1`, id)
		}
		if name == "Digi oc" {
			sqlMust(t, db, `UPDATE accounts SET provider_error_state='PROXY_ERROR',provider_error_detail='private-secret-canary',runtime_status='PROVIDER_PROXY_ERROR' WHERE id=$1`, id)
			if err := capacity.RecordCreateBlock(context.Background(), db, id, trialDenied()); err != nil {
				t.Fatal(err)
			}
			if b, err := capacity.ReadCreateBlock(context.Background(), db, id); err != nil || b == nil {
				t.Fatal("missing restriction fixture", err)
			}
			state, err := (&Server{DB: db}).accountBuildState(context.Background(), id)
			if err != nil || state.State != "LOCKED" || state.ProviderCanCreate || !strings.Contains(state.Reason, "Explicit provider account lock") {
				t.Fatal(state, err)
			}
		}
		if name == "Digi oc" {
			sqlMust(t, db, `UPDATE accounts SET provider='digitalocean',provider_state='ACTIVE',runtime_status='ISOLATION_WAIT' WHERE id=$1`, id)
			putBuildSnapshot(t, db, id, "locked", true, 3, 3)
			state, err := (&Server{DB: db}).accountBuildState(context.Background(), id)
			if err != nil || state.State != "LOCKED" || state.ProviderCanCreate || !strings.Contains(state.Reason, "Independent observation error") {
				t.Fatal(state, err)
			}
		}
		if name == "Oceano new1" {
			putBuildSnapshot(t, db, id, "warning", true, 10, 10)
			sqlMust(t, db, `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Account,Condition}','"CAPACITY_FULL"') WHERE account_id=$1`, id)
		}
	}
	var raw []byte
	if err := db.QueryRow(dashboardCountsSQL).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var d struct {
		Broken   int                                                         `json:"broken_accounts"`
		Degraded int                                                         `json:"degraded_accounts"`
		Healthy  int                                                         `json:"healthy_accounts"`
		Capacity int                                                         `json:"capacity_full_accounts"`
		Total    int                                                         `json:"account_health_total"`
		Rows     []struct{ Name, Health, Reason, ProviderErrorState string } `json:"account_health"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	if d.Broken != 3 || d.Degraded != 0 || d.Healthy != 3 || d.Capacity != 1 || d.Total != 7 || len(d.Rows) != 7 {
		t.Fatal(string(raw))
	}
	if strings.Contains(string(raw), "private-secret-canary") {
		t.Fatal("raw error detail leaked")
	}
	var rows []map[string]any
	var all map[string]json.RawMessage
	json.Unmarshal(raw, &all)
	json.Unmarshal(all["account_health"], &rows)
	for _, row := range rows {
		if row["name"] == "Digi oc" && (row["reason"] != "LOCKED" || row["provider_error_state"] != "PROXY_ERROR") {
			t.Fatal(row)
		}
	}
	id := seedBuildAccount(t, db, "digitalocean")
	putBuildSnapshot(t, db, id, "active", true, 5, 1)
	sqlMust(t, db, `UPDATE accounts SET provider_error_state='TRANSPORT_ERROR',runtime_status='PROVIDER_TRANSPORT_ERROR' WHERE id=$1`, id)
	if err := db.QueryRow(dashboardCountsSQL).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(raw, &d)
	if d.Broken != 3 || d.Degraded != 1 || d.Healthy != 3 {
		t.Fatal("transient observation marked healthy or hard-broken", string(raw))
	}
}

func TestAccountHealthRetainedLockRespectsCredentialAndResolution(t *testing.T) {
	db := adminTestDB(t)
	id := seedBuildAccount(t, db, "digitalocean")
	putBuildSnapshot(t, db, id, "locked", true, 3, 3)
	sqlMust(t, db, `UPDATE accounts SET provider_state='ACTIVE',provider_error_state='PROXY_ERROR',runtime_status='ISOLATION_WAIT' WHERE id=$1`, id)
	check := func(want string) {
		t.Helper()
		var raw []byte
		if e := db.QueryRow(dashboardCountsSQL).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		var d struct {
			Rows []struct{ Reason string } `json:"account_health"`
		}
		if e := json.Unmarshal(raw, &d); e != nil {
			t.Fatal(e)
		}
		if len(d.Rows) != 1 || d.Rows[0].Reason != want {
			t.Fatal(string(raw))
		}
	}
	check("LOCKED")
	sqlMust(t, db, `INSERT INTO secrets(id,account_id,kind,ciphertext,nonce,key_version,updated_at) SELECT secret_ref,id,'fixture','x','x',1,clock_timestamp() FROM accounts WHERE id=$1`, id)
	check("PROXY_ERROR")
	sqlMust(t, db, `UPDATE secrets SET updated_at=now()-interval '1 day' WHERE account_id=$1`, id)
	sqlMust(t, db, `UPDATE accounts SET provider_account_status='active',provider_account_observed_at=clock_timestamp() WHERE id=$1`, id)
	check("PROXY_ERROR")
}

func publishBuildObservation(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	ctx := context.Background()
	c := app.Container{DB: db}
	cfg, e := (app.Repository{DB: db}).Account(ctx, id)
	if e != nil {
		t.Fatal(e)
	}
	a, e := c.BeginProviderAttempt(ctx, id, false)
	if e != nil {
		t.Fatal(e)
	}
	var raw []byte
	if e = db.QueryRow(`SELECT canonical FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, id).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var obs providers.Observation
	if e = json.Unmarshal(raw, &obs); e != nil {
		t.Fatal(e)
	}
	if ok, e := c.PublishProviderObservation(ctx, app.AccountRuntime{Config: cfg}, a, obs, "READY", "fixture full observation"); !ok || e != nil {
		t.Fatal(ok, e)
	}
}
