package main

import (
	"context"
	"database/sql"
	_ "github.com/lib/pq"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIdentityProviderBulkPostgresPoolHeadroom(t *testing.T) {
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	if !strings.Contains(dsn, "bulk_test") {
		t.Fatal("test database required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(24)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	role, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer role.Close()
	parent, bulk, provider, identity := newControlWorkBudgets()
	scheduler := newSchedulerWorkBudget(parent)
	// Worst admitted call graph under the shared five-slot limit: bulk6,
	// provider2, two identity3, scheduler3 and role1. Each owner then needs one SQL connection.
	type lane struct {
		held  int
		admit func(context.Context, func(context.Context) error) error
	}
	lanes := []lane{{6, bulk.Do}, {2, provider.Do}, {3, identity.Do}, {3, identity.Do}, {3, scheduler.Do}}
	held := make(chan struct{}, 5)
	query := make(chan struct{})
	release := make(chan struct{})
	errs := make(chan error, 5)
	var wg sync.WaitGroup
	for _, l := range lanes {
		wg.Add(1)
		go func(l lane) {
			defer wg.Done()
			errs <- l.admit(ctx, func(ctx context.Context) error {
				var conns []*sql.Conn
				defer func() {
					for _, c := range conns {
						c.Close()
					}
				}()
				for range l.held {
					c, e := db.Conn(ctx)
					if e != nil {
						return e
					}
					conns = append(conns, c)
				}
				held <- struct{}{}
				select {
				case <-query:
				case <-ctx.Done():
					return ctx.Err()
				}
				if _, e := db.ExecContext(ctx, "SELECT pg_sleep(0.02)"); e != nil {
					return e
				}
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
		}(l)
	}
	defer func() { cancel(); wg.Wait() }()
	for range 5 {
		select {
		case <-held:
		case <-ctx.Done():
			t.Fatal("pool stalled admitting owners")
		}
	}
	if n := db.Stats().InUse; n != 18 {
		t.Fatal("unexpected long owner count", n)
	}
	close(query)
	for range 3 {
		if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
			t.Fatal("supervision headroom", err)
		}
	}
	close(release)
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if s := db.Stats(); s.WaitCount != 0 || s.InUse != 1 {
		t.Fatal("pool wait/leak", s)
	}
}
