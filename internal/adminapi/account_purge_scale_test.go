package adminapi

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
)

// Mirrors a retired account with hundreds of historical panels and a large
// shared inventory history. Queueing stays fast; each worker pass remains bounded.
func TestAccountPurgeHistoricalScale(t *testing.T) {
	if os.Getenv("DOB_PURGE_SCALE") != "1" {
		t.Skip("large-history opt-in")
	}
	db := adminTestDB(t)
	a, _, _ := seedPurgeDependencies(t, db)
	other, _, otherPanel := seedPanel(t, db, "http://unrelated.test")
	sqlMust(t, db, `WITH ds AS (
 INSERT INTO droplets(id,account_id,provider_resource_id,state,expires_at)
 SELECT gen_random_uuid(),$1,'history-'||n,'DELETED',now()-interval '1 day'
 FROM generate_series(1,434) n RETURNING id)
 INSERT INTO panel_instances(account_id,droplet_id,driver,base_url,auth_secret_ref,enabled)
 SELECT $1,id,'sanaei-3x-ui','http://history.test','fixture',false FROM ds`, a)
	// Bound fixture transaction memory while keeping the same 970,000 rows.
	for first := 1; first <= 2000; first += 100 {
		sqlMust(t, db, `INSERT INTO panel_inventory_syncs(panel_id,state,finished_at)
 SELECT p.id,CASE WHEN n%100=0 THEN 'RUNNING' ELSE 'COMPLETED' END,
 CASE WHEN n%100=0 THEN NULL ELSE now()-interval '1 day' END
 FROM panel_instances p CROSS JOIN generate_series($2::integer,$3::integer) n WHERE p.account_id=$1`, a, first, first+99)
	}
	sqlMust(t, db, `INSERT INTO panel_inventory_syncs(panel_id,state,finished_at)
 SELECT $1,'COMPLETED',now() FROM generate_series(1,100000)`, otherPanel)
	t.Log("970000-row history fixture seeded")
	sqlMust(t, db, "ANALYZE panel_instances")
	sqlMust(t, db, "ANALYZE panel_inventory_syncs")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	started := time.Now()
	err := app.QueueSavedAccountPurge(ctx, db, a, "LOCKED", 1, purgeConfirmation(t, db, a))
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("queue request blocked on historical cleanup")
	}
	passes := 0
	for {
		var remaining int
		if err = db.QueryRowContext(ctx, "SELECT count(*) FROM accounts WHERE id=$1", a).Scan(&remaining); err != nil {
			t.Fatal(err)
		}
		if remaining == 0 {
			break
		}
		sqlMust(t, db, "UPDATE account_purge_requests SET next_attempt_at=now() WHERE account_id=$1", a)
		pass := time.Now()
		processed, e := (app.Container{DB: db}).ProcessSavedAccountPurge(ctx)
		if e != nil || !processed {
			t.Fatal("native queued purge did not progress", processed, e)
		}
		if time.Since(pass) >= 30*time.Second {
			t.Fatal("worker pass exhausted deadline")
		}
		passes++
	}
	t.Logf("435 historical panels, 970000 shared inventory rows: native passes=%d elapsed=%s", passes, time.Since(started))

	var n int
	if err = db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", a).Scan(&n); err != nil || n != 0 {
		t.Fatal("account remains", n, err)
	}
	if err = db.QueryRow("SELECT count(*) FROM panel_inventory_syncs").Scan(&n); err != nil || n != 100000 {
		t.Fatal("history scope", n, err)
	}
	if err = db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", other).Scan(&n); err != nil || n != 1 {
		t.Fatal("other account removed", n, err)
	}
}
