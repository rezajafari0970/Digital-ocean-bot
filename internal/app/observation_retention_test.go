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
