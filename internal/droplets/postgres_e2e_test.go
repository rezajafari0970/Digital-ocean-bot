package droplets

import (
	"context"
	"database/sql"
	_ "github.com/lib/pq"
	"os"
	"testing"
	"time"
)

func TestRetiringWithoutExpiryIsDueE2E(t *testing.T) {
	dsn := os.Getenv("DOB_E2E_DSN")
	if dsn == "" {
		t.Skip("DOB_E2E_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	var aid, pid, did string
	if err = db.QueryRowContext(ctx, `INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'digitalocean','life-e2e','x') RETURNING id::text`).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), `DELETE FROM accounts WHERE id=$1`, aid)
	if err = db.QueryRowContext(ctx, `INSERT INTO deployment_profiles(id,account_id,name,config) VALUES(gen_random_uuid(),$1,'life','{}') RETURNING id::text`, aid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `INSERT INTO droplets(id,account_id,profile_id,provider_resource_id,state,profile,ready_at,expires_at) VALUES(gen_random_uuid(),$1,$2,'999','RETIRING','{}',NULL,NULL) RETURNING id::text`, aid, pid).Scan(&did); err != nil {
		t.Fatal(err)
	}
	items, err := (LifecycleStore{DB: db}).Due(ctx, time.Now().UTC(), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range items {
		if x.ID == did {
			return
		}
	}
	t.Fatalf("retiring droplet %s with null expiry was not due", did)
}
