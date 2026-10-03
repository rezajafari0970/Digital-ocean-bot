package app

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func fenceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "bulk_test") {
		t.Fatal("isolated database required")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := sanaei.UUIDv4()
	schema := "fence_" + strings.ReplaceAll(id, "-", "")
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	if _, err = db.Exec("CREATE TABLE accounts(id text PRIMARY KEY,enabled boolean,deletion_requested_at timestamptz); INSERT INTO accounts VALUES('test',true,NULL)"); err != nil {
		t.Fatal(err)
	}
	return db
}

type fenceTransport func(*http.Request) (*http.Response, error)

func (f fenceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMutationFenceWaitsForResponseAndBlocksNewCreates(t *testing.T) {
	db := fenceTestDB(t)
	ctx := context.Background()
	calls := 0
	transport := providerMutationFence{db: db, account: "test", base: fenceTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})}
	request, _ := http.NewRequest("POST", "https://provider.invalid/instances", nil)
	resp, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	locked := make(chan error, 1)
	go func() {
		tx, e := db.BeginTx(ctx, nil)
		if e != nil {
			locked <- e
			return
		}
		defer tx.Rollback()
		_, e = tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended('account-mutation:test',0))")
		if e == nil {
			_, e = tx.Exec("UPDATE accounts SET enabled=false,deletion_requested_at=now()")
		}
		if e == nil {
			e = tx.Commit()
		}
		locked <- e
	}()
	select {
	case e := <-locked:
		t.Fatal("deletion passed in-flight response", e)
	case <-time.After(100 * time.Millisecond):
	}
	resp.Body.Close()
	select {
	case e := <-locked:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deletion stuck")
	}
	if _, err = transport.RoundTrip(request); err == nil {
		t.Fatal("create after delete request")
	}
	if calls != 1 {
		t.Fatal("blocked create reached network")
	}
	for _, method := range []string{"GET", "DELETE"} {
		r, _ := http.NewRequest(method, request.URL.String(), nil)
		v, e := transport.RoundTrip(r)
		if e != nil {
			t.Fatal(e)
		}
		v.Body.Close()
	}
	if calls != 3 {
		t.Fatal(calls)
	}
}
func TestMutationFenceReleasesOnTransportLoss(t *testing.T) {
	db := fenceTestDB(t)
	f := providerMutationFence{db: db, account: "test", base: fenceTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("lost response") })}
	req, _ := http.NewRequest("POST", "https://provider.invalid/instances", nil)
	if _, err := f.RoundTrip(req); err == nil {
		t.Fatal("missing transport error")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended('account-mutation:test',0))"); err != nil {
		t.Fatal("lock leaked", err)
	}
}
