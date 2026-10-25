package app

import (
	"context"
	"testing"
)

func TestPreparedObservationRetentionKeepsEvidence(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	var panel string
	if err := db.QueryRow(`INSERT INTO panel_instances(account_id,droplet_id,driver,base_url,auth_secret_ref) VALUES($1,$2,'sanaei-3x-ui','http://fixture','unused') RETURNING id::text`, f.d.AccountID, f.d.DropletID).Scan(&panel); err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, `INSERT INTO panel_inventory_syncs(panel_id,state,finished_at) VALUES($1,'COMPLETED',now()-interval '10 days'),($1,'COMPLETED',now()),($1,'FAILED',now()-interval '20 days'),($1,'RUNNING',NULL)`, panel)
	execBootstrap(t, db, `INSERT INTO provider_snapshots(id,account_id,provider,data,canonical,created_at) VALUES(gen_random_uuid(),$1,'vultr','{}','{"Account":{"Status":"active"}}',now()-interval '10 days'),(gen_random_uuid(),$1,'vultr','{}','{"Account":{"Status":"active"}}',now()),(gen_random_uuid(),$1,'vultr','{}','{"Account":{"Status":"warning"}}',now()-interval '20 days')`, f.d.AccountID)
	n, err := (Container{DB: db}).PruneObservations(context.Background())
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM panel_inventory_syncs`).Scan(&count); err != nil || count != 3 {
		t.Fatal(count, err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM provider_snapshots`).Scan(&count); err != nil || count != 2 {
		t.Fatal(count, err)
	}
	n, err = (Container{DB: db}).PruneObservations(context.Background())
	if err != nil || n != 0 {
		t.Fatal(n, err)
	}
}

func TestObservationRetentionCatchupAndLimits(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	var panel string
	if err := db.QueryRow(`INSERT INTO panel_instances(account_id,droplet_id,driver,base_url,auth_secret_ref) VALUES($1,$2,'sanaei-3x-ui','http://fixture','unused') RETURNING id::text`, f.d.AccountID, f.d.DropletID).Scan(&panel); err != nil {
		t.Fatal(err)
	}
	// A seven-day rolling window can produce more than one thousand expired rows
	// per five-minute tick. Catch up without touching latest/failure evidence.
	execBootstrap(t, db, `INSERT INTO panel_inventory_syncs(panel_id,state,finished_at)
 SELECT $1,'COMPLETED',now()-interval '10 days' FROM generate_series(1,12500)`, panel)
	execBootstrap(t, db, `INSERT INTO panel_inventory_syncs(panel_id,state,finished_at)
 VALUES($1,'COMPLETED',now()),($1,'FAILED',now()-interval '20 days'),($1,'RUNNING',NULL)`, panel)
	execBootstrap(t, db, `INSERT INTO provider_snapshots(id,account_id,provider,data,canonical,created_at)
 SELECT gen_random_uuid(),$1,'vultr','{}','{"Account":{"Status":"active"}}',now()-interval '10 days' FROM generate_series(1,2500)`, f.d.AccountID)
	execBootstrap(t, db, `INSERT INTO provider_snapshots(id,account_id,provider,data,canonical,created_at)
 VALUES(gen_random_uuid(),$1,'vultr','{}','{"Account":{"Status":"active"}}',now()),(gen_random_uuid(),$1,'vultr','{}','{"Account":{"Status":"warning"}}',now()-interval '20 days')`, f.d.AccountID)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if n, err := (Container{DB: db}).PruneObservations(ctx); err == nil || n != 0 {
		t.Fatalf("cancelled retention mutated: %d %v", n, err)
	}
	n, err := (Container{DB: db}).PruneObservations(context.Background())
	if err != nil || n != 12500 {
		t.Fatalf("bounded catchup got %d %v; expected 10000 panel + 2500 provider", n, err)
	}
	var panels, providers int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM panel_inventory_syncs),(SELECT count(*) FROM provider_snapshots)`).Scan(&panels, &providers); err != nil || panels != 2503 || providers != 2 {
		t.Fatal(panels, providers, err)
	}
	n, err = (Container{DB: db}).PruneObservations(context.Background())
	if err != nil || n != 2500 {
		t.Fatal(n, err)
	}
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM panel_inventory_syncs),(SELECT count(*) FROM provider_snapshots)`).Scan(&panels, &providers); err != nil || panels != 3 || providers != 2 {
		t.Fatal(panels, providers, err)
	}
}
