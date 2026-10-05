package adminapi

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/residentialsync"
	"sync"
	"testing"
)

func TestRoutingProofLanesAreDisjointAndDrainServingFleet(t *testing.T) {
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
	// Even while the first due row remains due, no other lane selects it.
	first := map[string]bool{}
	for shard := 0; shard < 8; shard++ {
		p, ok, err := svc.NextDuePanelShard(ctx, true, shard, 8)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			if first[p.ID] || !expected[p.ID] {
				t.Fatal("overlapping lanes", p.ID)
			}
			first[p.ID] = true
		}
	}
	var wg sync.WaitGroup
	seen := make(chan string, 38)
	errs := make(chan error, 8)
	for shard := 0; shard < 8; shard++ {
		wg.Add(1)
		go func(shard int) {
			defer wg.Done()
			for n := 0; n <= 38; n++ {
				p, ok, err := svc.NextDuePanelShard(ctx, true, shard, 8)
				if err != nil {
					errs <- err
					return
				}
				if !ok {
					return
				}
				if _, err = db.Exec("INSERT INTO panel_routing_state(panel_id,state,revision,next_check_at) SELECT $1,'APPLIED',revision,now()+interval '1 hour' FROM residential_routing_control", p.ID); err != nil {
					errs <- err
					return
				}
				seen <- p.ID
			}
			errs <- fmt.Errorf("lane did not drain")
		}(shard)
	}
	wg.Wait()
	close(seen)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for id := range seen {
		if got[id] || !expected[id] {
			t.Fatal("duplicate or retired", id)
		}
		got[id] = true
	}
	if len(got) != 38 {
		t.Fatal("serving fleet not drained", len(got))
	}
	p, ok, err := svc.NextDuePanel(ctx, false)
	if err != nil || !ok || p.ID != dead {
		t.Fatal("retiring lane changed", p, ok, err)
	}
	for _, x := range [][2]int{{-1, 8}, {8, 8}, {0, 0}, {0, 9}} {
		if _, _, err = svc.NextDuePanelShard(ctx, true, x[0], x[1]); err == nil {
			t.Fatal("invalid lane accepted", x)
		}
	}
}
