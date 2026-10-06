package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
)

func ruleFixture(t *testing.T, db *sql.DB, provider string, n int) (string, []string) {
	t.Helper()
	a := seedBuildAccount(t, db, provider)
	sqlMust(t, db, `UPDATE accounts SET desired_server_count=$2,preferred_region='ams',
 preferred_regions='["ams"]',preferred_sizes='["small"]',preferred_image='ubuntu-24-04-x64',
 preferred_images='["ubuntu-24-04-x64"]',server_lifetime_seconds=3600,server_lifetime_min_seconds=3600,
 server_lifetime_max_seconds=5400,build_spacing_minutes=1,build_spacing_max_minutes=3 WHERE id=$1`, a, n)
	sqlMust(t, db, `INSERT INTO installers(name,version,manifest,sha256,active)
 VALUES('rule-test',1,'{"capabilities":["xui_database","xui_panel"]}',repeat('a',64),true)
 ON CONFLICT(name,version) DO NOTHING`)
	putBuildSnapshot(t, db, a, "active", true, 100, n)
	var ids []string
	for i := 0; i < n; i++ {
		d := perfUUID()
		sqlMust(t, db, `INSERT INTO droplets(id,account_id,provider_resource_id,state,expires_at)
  VALUES($1,$2,$3,'READY',now()+make_interval(mins=>$4))`, d, a, "fixture-"+d, 60+i)
		ids = append(ids, d)
	}
	return a, ids
}
func ruleSave(t *testing.T, db *sql.DB, a string, apply bool, desired, min, max int) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"name": "rule fixture", "regions": []string{"ams"}, "sizes": []string{"small"},
		"image": "ubuntu-24-04-x64", "images": []string{"ubuntu-24-04-x64"}, "network_mode": "direct", "lifetime_min_seconds": min,
		"lifetime_max_seconds": max, "build_spacing_minutes": 1, "build_spacing_max_minutes": 3,
		"desired_server_count": desired, "max_concurrent": 3, "fallback_any_region": true, "apply_to_existing": apply})
	r := httptest.NewRequest("PUT", "/", strings.NewReader(string(body)))
	r.SetPathValue("id", a)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin}))
	w := httptest.NewRecorder()
	(&Server{DB: db}).updateAccount(w, r)
	if w.Code != 200 {
		t.Fatalf("save %d %s", w.Code, w.Body.String())
	}
	return w
}
func ruleCount(t *testing.T, db *sql.DB, a, state string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM droplets WHERE account_id=$1 AND state=$2", a, state).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func ruleTick(t *testing.T, db *sql.DB, a string) {
	t.Helper()
	if err := (app.Container{DB: db}).ProcessAccountRuleApplication(context.Background(), a); err != nil {
		t.Fatal(err)
	}
}
func TestAccountRuleApplicationFutureOnlyAndDesiredMatrix(t *testing.T) {
	db := adminTestDB(t)
	for _, provider := range []string{"digitalocean", "vultr", "upcloud"} {
		t.Run(provider, func(t *testing.T) {
			a, ids := ruleFixture(t, db, provider, 15)
			var expiry time.Time
			if err := db.QueryRow("SELECT expires_at FROM droplets WHERE id=$1", ids[0]).Scan(&expiry); err != nil {
				t.Fatal(err)
			}
			ruleSave(t, db, a, false, 15, 9000, 9600)
			ruleTick(t, db, a)
			var got time.Time
			if err := db.QueryRow("SELECT expires_at FROM droplets WHERE id=$1", ids[0]).Scan(&got); err != nil || !got.Equal(expiry) {
				t.Fatal("expiry changed", got, err)
			}
			if ruleCount(t, db, a, "READY") != 15 {
				t.Fatal("future-only touched old servers")
			}
			// Desired-only growth with ON has no rollout, even with earlier future-only revisions.
			ruleSave(t, db, a, true, 16, 9000, 9600)
			ruleTick(t, db, a)
			var target sql.NullInt64
			var revision int
			if err := db.QueryRow("SELECT replacement_revision,revision FROM account_rule_application WHERE account_id=$1", a).Scan(&target, &revision); err != nil {
				t.Fatal(err)
			}
			if target.Valid || revision != 1 || ruleCount(t, db, a, "RETIRING") != 0 {
				t.Fatal("growth rotated old servers", target, revision)
			}
			var deficit int
			if err := db.QueryRow("SELECT desired_server_count-(SELECT count(*) FROM droplets WHERE account_id=$1 AND state<>'DELETED') FROM accounts WHERE id=$1", a).Scan(&deficit); err != nil || deficit != 1 {
				t.Fatal("growth deficit", deficit, err)
			}
			// OFF reduction waits for expiry.
			ruleSave(t, db, a, false, 14, 9000, 9600)
			ruleTick(t, db, a)
			if ruleCount(t, db, a, "READY") != 15 {
				t.Fatal("OFF shrink deleted early")
			}
			// Reset target then request ON reduction.
			ruleSave(t, db, a, true, 15, 9000, 9600)
			ruleSave(t, db, a, true, 14, 9000, 9600)
			var wg sync.WaitGroup
			errs := make(chan error, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					errs <- (app.Container{DB: db}).ProcessAccountRuleApplication(context.Background(), a)
				}()
			}
			wg.Wait()
			close(errs)
			for e := range errs {
				if e != nil {
					t.Fatal(e)
				}
			}
			if ruleCount(t, db, a, "RETIRING") != 1 || ruleCount(t, db, a, "READY") != 14 {
				t.Fatal("shrink was not exactly one")
			}
			var selected string
			if err := db.QueryRow("SELECT waiting_droplet_id::text FROM account_rule_application WHERE account_id=$1", a).Scan(&selected); err != nil || selected != ids[0] {
				t.Fatal("not earliest expiry", selected, err)
			}
			// Provider confirmation simulation: no external provider call in this test.
			sqlMust(t, db, "UPDATE droplets SET state='DELETING' WHERE id=$1", selected)
			if err := (app.Container{DB: db}).ConfirmDeleted(context.Background(), a, "fixture-"+selected); err != nil {
				t.Fatal(err)
			}
			ruleTick(t, db, a)
			if ruleCount(t, db, a, "READY") != 14 || ruleCount(t, db, a, "RETIRING") != 0 {
				t.Fatal("shrink rotated another server")
			}
		})
	}
}
func TestAccountRuleApplicationRolloutDuplicateRestartAndOff(t *testing.T) {
	db := adminTestDB(t)
	a, ids := ruleFixture(t, db, "digitalocean", 3)
	ruleSave(t, db, a, true, 3, 9000, 9600)
	ruleSave(t, db, a, true, 3, 9000, 9600)
	var rev int
	if err := db.QueryRow("SELECT revision FROM account_rule_application WHERE account_id=$1", a).Scan(&rev); err != nil || rev != 1 {
		t.Fatal("duplicate revision", rev, err)
	}
	ruleTick(t, db, a)
	ruleTick(t, db, a)
	if ruleCount(t, db, a, "RETIRING") != 1 {
		t.Fatal("duplicate claim")
	}
	var next time.Time
	if err := db.QueryRow("SELECT next_action_at FROM account_rule_application WHERE account_id=$1", a).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if d := time.Until(next); d < 55*time.Second || d > 181*time.Second {
		t.Fatal("spacing", d)
	}
	sqlMust(t, db, "UPDATE droplets SET state='DELETING' WHERE id=$1", ids[0])
	if err := (app.Container{DB: db}).ConfirmDeleted(context.Background(), a, "fixture-"+ids[0]); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "UPDATE account_rule_application SET next_action_at=now()-interval '1 minute' WHERE account_id=$1", a)
	ruleTick(t, db, a)
	if ruleCount(t, db, a, "RETIRING") != 0 {
		t.Fatal("deleted next before recovery")
	}
	// A newly ready replacement restores the slot and carries current rules.
	d, p := perfUUID(), perfUUID()
	sqlMust(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$2,'replacement','{}')", p, a)
	sqlMust(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state,expires_at) VALUES($1,$2,$3,'READY',now()+interval '155 minutes')", d, a, "fixture-"+d)
	sqlMust(t, db, "INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step,build_rules_revision) VALUES(gen_random_uuid(),$1,$2,$3,'PANEL_COMPLETE','done',1)", a, p, d)
	ruleTick(t, db, a)
	if ruleCount(t, db, a, "RETIRING") != 1 {
		t.Fatal("durable resume did not advance")
	}
	var chosen string
	if err := db.QueryRow("SELECT waiting_droplet_id::text FROM account_rule_application WHERE account_id=$1", a).Scan(&chosen); err != nil || chosen != ids[1] {
		t.Fatal("new revision selected", chosen, err)
	}
	ruleSave(t, db, a, false, 3, 9000, 9600)
	ruleTick(t, db, a)
	var enabled bool
	var target sql.NullInt64
	if err := db.QueryRow("SELECT apply_to_existing,replacement_revision FROM account_rule_application WHERE account_id=$1", a).Scan(&enabled, &target); err != nil || enabled || target.Valid {
		t.Fatal("OFF did not cancel unstarted work", err)
	}
}
func TestAccountRuleApplicationBlockedAndTransactionRollback(t *testing.T) {
	db := adminTestDB(t)
	a, _ := ruleFixture(t, db, "upcloud", 2)
	ruleSave(t, db, a, true, 2, 9000, 9600)
	sqlMust(t, db, "INSERT INTO account_create_blocks(account_id,code) VALUES($1,'TRIAL_FIREWALL')", a)
	ruleTick(t, db, a)
	if ruleCount(t, db, a, "READY") != 2 {
		t.Fatal("create-denied account retired")
	}
	sqlMust(t, db, "DELETE FROM account_create_blocks WHERE account_id=$1", a)
	// An interrupted Save rolls back both policy and replacement intent.
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	before, desired, e := app.AccountBuildRulesTx(context.Background(), tx, a)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec("UPDATE accounts SET server_lifetime_min_seconds=10000 WHERE id=$1", a); e != nil {
		t.Fatal(e)
	}
	if e = app.SaveAccountRuleApplicationTx(context.Background(), tx, a, before, desired, false); e != nil {
		t.Fatal(e)
	}
	tx.Rollback()
	var min int
	var apply bool
	if e = db.QueryRow("SELECT a.server_lifetime_min_seconds,p.apply_to_existing FROM accounts a JOIN account_rule_application p ON p.account_id=a.id WHERE a.id=$1", a).Scan(&min, &apply); e != nil || min != 9000 || !apply {
		t.Fatal(min, apply, e)
	}
	// Stale capacity, disabled accounts and unknown work cannot retire a server.
	sqlMust(t, db, "UPDATE provider_snapshots SET created_at=now()-interval '10 minutes' WHERE account_id=$1", a)
	ruleTick(t, db, a)
	if ruleCount(t, db, a, "READY") != 2 {
		t.Fatal("stale snapshot retired")
	}
	putBuildSnapshot(t, db, a, "active", true, 100, 2)
	sqlMust(t, db, "UPDATE accounts SET enabled=false WHERE id=$1", a)
	ruleTick(t, db, a)
	if ruleCount(t, db, a, "READY") != 2 {
		t.Fatal("disabled account retired")
	}
}
func TestAccountRuleApplicationFutureOnlyListAndNoOp(t *testing.T) {
	db := adminTestDB(t)
	a, _ := ruleFixture(t, db, "digitalocean", 1)
	ruleSave(t, db, a, false, 1, 3600, 5400)
	var rev int
	if e := db.QueryRow("SELECT revision FROM account_rule_application WHERE account_id=$1", a).Scan(&rev); e != nil || rev != 0 {
		t.Fatal("no-op revision", rev, e)
	}
	w := httptest.NewRecorder()
	(&Server{DB: db}).accounts(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var list []map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &list); e != nil {
		t.Fatal(e)
	}
	if len(list) != 1 || fmt.Sprint(list[0]["rule_application"]) == "" {
		t.Fatal("missing control state")
	}
	p := list[0]["rule_application"].(map[string]any)
	if p["apply_to_existing"] != false {
		t.Fatal("default not OFF", p)
	}
}

func TestAccountRuleApplicationOffCancelsUnstartedAndFencesStaleWorker(t *testing.T) {
	db := adminTestDB(t)
	a, ids := ruleFixture(t, db, "digitalocean", 3)
	ruleSave(t, db, a, true, 3, 9000, 9600)
	ruleTick(t, db, a)
	ruleSave(t, db, a, false, 3, 9000, 9600)
	if ruleCount(t, db, a, "RETIRING") != 0 || ruleCount(t, db, a, "READY") != 3 {
		t.Fatal("OFF left an unstarted retirement")
	}
	allowed, err := (app.Container{DB: db}).AdmitAccountRuleRetirement(context.Background(), a, ids[0])
	if err != nil || allowed {
		t.Fatal("stale worker may delete restored server", allowed, err)
	}
	// An admitted action remains reconciliable; OFF cannot undo a sent delete.
	ruleSave(t, db, a, true, 3, 10000, 11000)
	ruleTick(t, db, a)
	allowed, err = (app.Container{DB: db}).AdmitAccountRuleRetirement(context.Background(), a, ids[0])
	if err != nil || !allowed {
		t.Fatal("admission", allowed, err)
	}
	ruleSave(t, db, a, false, 3, 10000, 11000)
	if ruleCount(t, db, a, "RETIRING") != 1 {
		t.Fatal("lost already-started deletion")
	}
}
func TestAccountRuleApplicationShrinkChoosesExpiringAndGrowthCancelsQueue(t *testing.T) {
	db := adminTestDB(t)
	a, ids := ruleFixture(t, db, "vultr", 3)
	sqlMust(t, db, "UPDATE droplets SET state='EXPIRING',expires_at=now()-interval '1 second' WHERE id=$1", ids[0])
	ruleSave(t, db, a, true, 2, 3600, 5400)
	ruleTick(t, db, a)
	var chosen string
	if err := db.QueryRow("SELECT waiting_droplet_id::text FROM account_rule_application WHERE account_id=$1", a).Scan(&chosen); err != nil || chosen != ids[0] {
		t.Fatal("ignored earliest EXPIRING server", chosen, err)
	}
	// Raising Desired before dispatch cancels the selected shrink.
	ruleSave(t, db, a, true, 3, 3600, 5400)
	var state string
	if err := db.QueryRow("SELECT state FROM droplets WHERE id=$1", ids[0]).Scan(&state); err != nil || state != "EXPIRING" {
		t.Fatal("did not restore original state", state, err)
	}
	if ruleCount(t, db, a, "RETIRING") != 0 {
		t.Fatal("growth retained an obsolete shrink")
	}
}
func TestAccountRuleApplicationCompoundGrowthDoesNotRetireAndCadenceOnlyDoesNotRotate(t *testing.T) {
	db := adminTestDB(t)
	a, _ := ruleFixture(t, db, "digitalocean", 3)
	ruleSave(t, db, a, true, 4, 9000, 9600)
	ruleTick(t, db, a)
	if ruleCount(t, db, a, "RETIRING") != 0 {
		t.Fatal("compound growth deleted capacity before filling deficit")
	}
	ruleSave(t, db, a, false, 3, 9000, 9600)
	ctx := context.Background()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	before, desired, err := app.AccountBuildRulesTx(ctx, tx, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec("UPDATE accounts SET build_spacing_minutes=4,build_spacing_max_minutes=5,auto_max_concurrent=2 WHERE id=$1", a); err != nil {
		t.Fatal(err)
	}
	if err = app.SaveAccountRuleApplicationTx(ctx, tx, a, before, desired, true); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	ruleTick(t, db, a)
	if ruleCount(t, db, a, "RETIRING") != 0 {
		t.Fatal("cadence-only change rotated servers")
	}
}
