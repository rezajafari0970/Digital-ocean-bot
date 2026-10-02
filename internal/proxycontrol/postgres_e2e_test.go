package proxycontrol

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

func proxyE2EDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DOB_E2E_DSN")
	if dsn == "" {
		t.Skip("DOB_E2E_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func proxyE2EFixture(t *testing.T, db *sql.DB) (string, string, string) {
	t.Helper()
	ctx := context.Background()
	provider := "digitalocean"
	var accountID, proxyID string
	if err := db.QueryRowContext(ctx, "INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),$1,'proxy-control-e2e','x') RETURNING id::text", provider).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), "DELETE FROM accounts WHERE id=$1", accountID) })
	if err := db.QueryRowContext(ctx, "INSERT INTO proxies(id,name,type,host,port,status) VALUES(gen_random_uuid(),'proxy-control-e2e','http','127.0.0.1',18080,'healthy') RETURNING id::text").Scan(&proxyID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), "DELETE FROM proxies WHERE id=$1", proxyID) })
	return accountID, proxyID, provider
}

func TestSQLStoreHalfOpenLeaseExclusivePostgresE2E(t *testing.T) {
	db := proxyE2EDB(t)
	accountID, proxyID, provider := proxyE2EFixture(t, db)
	ctx := context.Background()
	store := SQLStore{DB: db}
	now := time.Now().UTC()
	retry := now.Add(-time.Second)
	if _, err := db.ExecContext(ctx, "INSERT INTO proxy_runtime_state(account_id,proxy_id,provider,health_state,circuit_state,retry_after) VALUES($1,$2,$3,'down','open',$4)", accountID, proxyID, provider, retry); err != nil {
		t.Fatal(err)
	}
	const workers = 12
	start := make(chan struct{})
	var allowed atomic.Int32
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, ok, err := store.Acquire(ctx, accountID, proxyID, provider, now, 30*time.Second)
			if ok {
				allowed.Add(1)
			}
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := allowed.Load(); got != 1 {
		t.Fatalf("allowed probes=%d want=1", got)
	}
}

func TestSQLStoreGenerationAndStaleObservationPostgresE2E(t *testing.T) {
	db := proxyE2EDB(t)
	accountID, proxyID, provider := proxyE2EFixture(t, db)
	ctx := context.Background()
	store := SQLStore{DB: db}
	g1, err := store.BumpGeneration(ctx, accountID, proxyID, provider)
	if err != nil {
		t.Fatal(err)
	}
	g2, err := store.BumpGeneration(ctx, accountID, proxyID, provider)
	if err != nil {
		t.Fatal(err)
	}
	if g2 != g1+1 {
		t.Fatalf("generation %d -> %d", g1, g2)
	}
	result := network.HealthResult{Status: network.StatusDown, CheckedAt: time.Now().UTC(), Error: "stale injected failure"}
	_, applied, err := store.ApplyObservationForGeneration(ctx, accountID, proxyID, provider, g1, result, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Fatal("stale observation applied")
	}
	current, found, err := store.CurrentGeneration(ctx, accountID, proxyID, provider)
	if err != nil || !found || current != g2 {
		t.Fatalf("current=%d found=%v err=%v", current, found, err)
	}
}
