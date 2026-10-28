package app

import (
	"context"
	"testing"
	"time"
)

func TestProxyKeeperLockSerializesAcrossConnectionsPostgresE2E(t *testing.T) {
	db := installerBootstrapDB(t)
	c, a, _ := countryFixture(t, db, "")
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first, ok, err := c.acquireProxyKeeperLock(ctx, a)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	defer releaseProxyKeeperLock(first)
	second, ok, err := c.acquireProxyKeeperLock(ctx, a)
	if err != nil || ok {
		releaseProxyKeeperLock(second)
		t.Fatal(ok, err)
	}
	releaseProxyKeeperLock(first)
	third, ok, err := c.acquireProxyKeeperLock(ctx, a)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	defer releaseProxyKeeperLock(third)
	stale := context.WithValue(ctx, proxyKeeperLeaseKey{}, first)
	if tx, err := countryRouteTx(stale, db, a); err == nil {
		tx.Rollback()
		t.Fatal("expired owner published")
	}
}
