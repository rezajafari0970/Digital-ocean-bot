package adminapi

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
)

// Mirrors a retired account with hundreds of historical panels and a large
// shared inventory history. This must complete within the original API budget.
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
	sqlMust(t, db, `INSERT INTO panel_inventory_syncs(panel_id,state,finished_at)
 SELECT p.id,CASE WHEN n%100=0 THEN 'RUNNING' ELSE 'COMPLETED' END,
 CASE WHEN n%100=0 THEN NULL ELSE now()-interval '1 day' END
 FROM panel_instances p CROSS JOIN generate_series(1,2000) n WHERE p.account_id=$1`, a)
	sqlMust(t, db, `INSERT INTO panel_inventory_syncs(panel_id,state,finished_at)
 SELECT $1,'COMPLETED',now() FROM generate_series(1,100000)`, otherPanel)
	sqlMust(t, db, "ANALYZE panel_instances")
	sqlMust(t, db, "ANALYZE panel_inventory_syncs")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	started := time.Now()
	err := app.PurgeSavedAccount(ctx, db, a, "LOCKED", 1, purgeConfirmation(t, db, a))
	t.Logf("435 historical panels, 970000 shared inventory rows: elapsed=%s error=%v", time.Since(started), err)
	if err != nil {
		t.Fatal(err)
	}
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
