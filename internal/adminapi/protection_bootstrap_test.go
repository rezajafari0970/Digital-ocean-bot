package adminapi

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/globalreality"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"testing"
	"time"
)

func TestProtectionBootstrapPartitionAndWithdrawal(t *testing.T) {
	db := adminTestDB(t)
	a, _, newID := seedPanel(t, db, "http://new.invalid")
	_, _, readyID := seedPanel(t, db, "http://ready.invalid")
	sqlMust(t, db, `INSERT INTO panel_inbound_inventory(panel_id,remote_id,protocol,port,transport,security,enabled,present,raw_hash,first_seen_at,last_seen_at) VALUES($1,1,'vless',443,'xhttp','reality',true,true,'fixture',now(),now())`, readyID)
	source := readyworker.SQLSource{DB: db}
	check := func(initial bool, want string) {
		t.Helper()
		xs, e := source.EligibleRealityPanels(context.Background(), initial)
		if e != nil {
			t.Fatal(e)
		}
		if want == "" {
			if len(xs) != 0 {
				t.Fatalf("unexpected eligible panels: %+v", xs)
			}
		} else if len(xs) != 1 || xs[0].ID != want {
			t.Fatalf("initial=%v panels=%+v want=%s", initial, xs, want)
		}
	}
	check(true, newID)
	check(false, readyID)
	sqlMust(t, db, "UPDATE accounts SET enabled=false WHERE id=$1", a)
	check(true, "")
	sqlMust(t, db, "UPDATE accounts SET enabled=true,deletion_requested_at=now() WHERE id=$1", a)
	check(true, "")
	sqlMust(t, db, "UPDATE accounts SET deletion_requested_at=NULL WHERE id=$1", a)
	check(true, newID)
	sqlMust(t, db, "UPDATE panel_inbound_inventory SET present=false WHERE panel_id=$1", readyID)
	check(false, "")
}
func TestPeriodicGlobalRealitySkipsBusyPanel(t *testing.T) {
	db := adminTestDB(t)
	_, _, id := seedPanel(t, db, "http://fixture.invalid")
	conn, e := db.Conn(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	_, e = conn.ExecContext(context.Background(), "SELECT pg_advisory_lock(hashtextextended($1,0))", "panel-config:"+id)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock_all()")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	e = (globalreality.Service{DB: db}).TryReconcilePanel(ctx, readyworker.Panel{ID: id}, false)
	if e != nil || time.Since(start) > 500*time.Millisecond {
		t.Fatalf("periodic work queued behind busy lock: %v", e)
	}
}
