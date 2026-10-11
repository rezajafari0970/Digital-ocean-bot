package adminapi

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/residentialsync"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"sync"
	"testing"
	"time"
)

func TestRoutingSharedQueueClaimsExcludeDuplicatesAndRetired(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	expected := map[string]bool{}
	for i := 0; i < 38; i++ {
		_, _, id := seedPanel(t, db, fmt.Sprintf("http://127.0.0.1:%d", i+1))
		expected[id] = true
	}
	_, deadDroplet, dead := seedPanel(t, db, "http://127.0.0.1:999")
	sqlMust(t, db, "UPDATE droplets SET state='RETIRING',expires_at=now()-interval '1 hour' WHERE id=$1", deadDroplet)
	sqlMust(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	svc := residentialsync.Service{DB: db}
	failures := worker.FailureStore{DB: db}
	seen := make(chan string, 38)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			after := ""
			for n := 0; n < 150; n++ {
				p, ok, e := svc.NextDuePanelForWorker(ctx, true, after)
				if e != nil {
					errs <- e
					return
				}
				if !ok {
					return
				}
				after = p.ID
				claimed, e := failures.ReserveOutcome(ctx, "residential_sync", p.ID, "", 90*time.Second)
				if e != nil {
					errs <- e
					return
				}
				if !claimed {
					continue
				}
				// Discovery can precede another consumer's completion. ReconcilePanel
				// rechecks due eligibility before native work; preserve that boundary here.
				var due bool
				if e = db.QueryRow("SELECT r.panel_id IS NULL OR r.revision<>c.revision OR r.next_check_at<=now() FROM residential_routing_control c LEFT JOIN panel_routing_state r ON r.panel_id=$1 WHERE c.singleton", p.ID).Scan(&due); e != nil {
					errs <- e
					return
				}
				if !due {
					if e = failures.RecordOutcome(ctx, "residential_sync", p.ID, "", nil); e != nil {
						errs <- e
						return
					}
					continue
				}
				if _, e = db.Exec("INSERT INTO panel_routing_state(panel_id,state,revision,next_check_at) SELECT $1,'APPLIED',revision,now()+interval '1 hour' FROM residential_routing_control", p.ID); e != nil {
					errs <- e
					return
				}
				if e = failures.RecordOutcome(ctx, "residential_sync", p.ID, "", nil); e != nil {
					errs <- e
					return
				}
				seen <- p.ID
			}
			errs <- fmt.Errorf("shared queue failed to drain")
		}()
	}
	wg.Wait()
	close(seen)
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	got := map[string]bool{}
	for id := range seen {
		if got[id] || !expected[id] {
			t.Fatal("duplicate or retired panel", id)
		}
		got[id] = true
	}
	if len(got) != len(expected) {
		t.Fatal("fleet not drained", len(got))
	}
	p, ok, e := svc.NextDuePanelForWorker(ctx, false, "")
	if e != nil || !ok || p.ID != dead {
		t.Fatal("retirement consumer changed", p, ok, e)
	}
}
