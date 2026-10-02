package app

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

func TestProxyKeeperLockSerializesAcrossConnectionsPostgresE2E(t *testing.T) {
	dsn := os.Getenv("DOB_E2E_DSN")
	if dsn == "" {
		t.Skip("DOB_E2E_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	c := Container{DB: db}
	ctx := context.Background()
	key := "keeper-e2e"

	conn1, ok, err := c.acquireProxyKeeperLock(ctx, key)
	if err != nil || !ok {
		t.Fatalf("first lock ok=%v err=%v", ok, err)
	}
	defer releaseProxyKeeperLock(conn1, key)

	conn2, ok, err := c.acquireProxyKeeperLock(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		releaseProxyKeeperLock(conn2, key)
		t.Fatal("second connection acquired an already-held keeper lock")
	}

	releaseProxyKeeperLock(conn1, key)
	conn1 = nil

	conn3, ok, err := c.acquireProxyKeeperLock(ctx, key)
	if err != nil || !ok {
		t.Fatalf("reacquire ok=%v err=%v", ok, err)
	}
	releaseProxyKeeperLock(conn3, key)
}
