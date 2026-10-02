package droplets

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestDuePrioritizesRetiringOverOlderExpiringE2E(t *testing.T) {
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

	var aid, pid, expiringID, retiringID string
	if err = db.QueryRowContext(ctx, `INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'digitalocean','life-priority-e2e','x') RETURNING id::text`).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), `DELETE FROM accounts WHERE id=$1`, aid)
	if err = db.QueryRowContext(ctx, `INSERT INTO deployment_profiles(id,account_id,name,config) VALUES(gen_random_uuid(),$1,'life','{}') RETURNING id::text`, aid).Scan(&pid); err != nil {
		t.Fatal(err)
	}

	if err = db.QueryRowContext(ctx, `INSERT INTO droplets(id,account_id,profile_id,provider_resource_id,state,profile,ready_at,expires_at,created_at,updated_at) VALUES(gen_random_uuid(),$1,$2,'exp','EXPIRING','{}',now()-interval '3 hours',now()-interval '2 hours',now()-interval '3 hours',now()-interval '2 hours') RETURNING id::text`, aid, pid).Scan(&expiringID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `INSERT INTO droplets(id,account_id,profile_id,provider_resource_id,state,profile,ready_at,expires_at,created_at,updated_at) VALUES(gen_random_uuid(),$1,$2,'ret','RETIRING','{}',now()-interval '1 hour',NULL,now()-interval '1 hour',now()-interval '1 hour') RETURNING id::text`, aid, pid).Scan(&retiringID); err != nil {
		t.Fatal(err)
	}

	items, err := (LifecycleStore{DB: db}).Due(ctx, time.Now().UTC(), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range items {
		if x.AccountID == aid {
			if x.ID != retiringID {
				t.Fatalf("due selected %s; want retiring %s (older expiring=%s must not block progress)", x.ID, retiringID, expiringID)
			}
			return
		}
	}
	t.Fatal("test account missing from due lifecycle items")
}
