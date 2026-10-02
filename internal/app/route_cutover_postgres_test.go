package app

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestRouteCutoverWaitsForInflightSharedLockPostgresE2E(t *testing.T) {
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

	var accountID, p1, p2 string
	if err := db.QueryRowContext(ctx, `INSERT INTO accounts(id,provider,name,secret_ref)
VALUES(gen_random_uuid(),'digitalocean','route-e2e-'||gen_random_uuid()::text,'x') RETURNING id::text`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), "DELETE FROM accounts WHERE id=$1", accountID)

	if err := db.QueryRowContext(ctx, `INSERT INTO proxies(id,name,type,host,port,status)
VALUES(gen_random_uuid(),'route-p1-'||gen_random_uuid()::text,'http','127.0.0.1',18080,'down') RETURNING id::text`).Scan(&p1); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), "DELETE FROM proxies WHERE id=$1", p1)
	if err := db.QueryRowContext(ctx, `INSERT INTO proxies(id,name,type,host,port,status)
VALUES(gen_random_uuid(),'route-p2-'||gen_random_uuid()::text,'http','127.0.0.1',18081,'healthy') RETURNING id::text`).Scan(&p2); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), "DELETE FROM proxies WHERE id=$1", p2)
	if _, err := db.ExecContext(ctx, `INSERT INTO network_profiles(id,account_id,mode,proxy_id)
VALUES(gen_random_uuid(),$1,'proxy_required',$2)`, accountID, p1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO account_proxy_pool(account_id,proxy_id,priority,enabled)
VALUES($1,$2,0,true),($1,$3,10,true)`, accountID, p1, p2); err != nil {
		t.Fatal(err)
	}

	c := Container{DB: db}
	routeConn, err := c.acquireAccountRouteSharedLock(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}

	cutCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	err = c.ensureActiveAccountProxy(cutCtx, accountID)
	cancel()
	if err == nil {
		releaseAccountRouteSharedLock(routeConn, accountID)
		t.Fatal("cutover committed while an old-route request lock was held")
	}

	var current string
	if err := db.QueryRowContext(ctx, `SELECT proxy_id::text FROM network_profiles WHERE account_id=$1`, accountID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if current != p1 {
		releaseAccountRouteSharedLock(routeConn, accountID)
		t.Fatalf("proxy changed under inflight route: got=%s want=%s", current, p1)
	}

	releaseAccountRouteSharedLock(routeConn, accountID)
	if err := c.ensureActiveAccountProxy(ctx, accountID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT proxy_id::text FROM network_profiles WHERE account_id=$1`, accountID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if current != p2 {
		t.Fatalf("proxy=%s want=%s", current, p2)
	}
}
