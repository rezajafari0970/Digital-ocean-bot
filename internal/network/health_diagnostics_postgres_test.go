package network

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Uses the actual migrations in a disposable schema on an explicit test DB.
func healthDiagnosticsDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "bulk_test") {
		t.Fatal("isolated bulk_test database required")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	var suffix string
	if err = admin.QueryRow("SELECT replace(gen_random_uuid()::text,'-','')").Scan(&suffix); err != nil {
		t.Fatal(err)
	}
	schema := "proxy_diagnostics_" + suffix
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
	dir := t.TempDir()
	files, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		raw = []byte(strings.ReplaceAll(string(raw), "tc.table_schema='public'", "tc.table_schema=current_schema()"))
		if err = os.WriteFile(filepath.Join(dir, filepath.Base(f)), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = (migrate.Runner{DB: db, Dir: dir}).Up(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestHealthDiagnosticsPersistencePostgres(t *testing.T) {
	db := healthDiagnosticsDB(t)
	var id string
	if err := db.QueryRow("INSERT INTO proxies(id,name,type,host,port,status) VALUES(gen_random_uuid(),'health-diagnostic','socks5','127.0.0.1',1,'healthy') RETURNING id::text").Scan(&id); err != nil {
		t.Fatal(err)
	}
	store := SQLHealthStore{DB: db}
	ctx := context.Background()
	checked := time.Now().UTC().Truncate(time.Microsecond)
	lastSuccess := checked.Add(-time.Minute)
	state := HealthState{Status: StatusHealthy, LastSuccessAt: lastSuccess}
	policy := HealthPolicy{FailureThreshold: 2, RecoveryThreshold: 2}
	check := func(wantCode, wantStatus string, latency int64) {
		t.Helper()
		var code, status string
		var ms int64
		var at, success time.Time
		err := db.QueryRow("SELECT COALESCE(health_error,''),status,latency_ms,last_checked_at,last_success_at FROM proxies WHERE id=$1", id).Scan(&code, &status, &ms, &at, &success)
		if err != nil {
			t.Fatal(err)
		}
		if code != wantCode || status != wantStatus || ms != latency || !at.Equal(state.LastCheckedAt) || !success.Equal(state.LastSuccessAt) {
			t.Fatalf("unexpected persisted observation code=%q status=%q ms=%d", code, status, ms)
		}
	}
	state = state.Apply(HealthResult{Status: StatusDown, CheckedAt: checked, Latency: 9 * time.Millisecond, Error: "PROXY_AUTH_FAILED"}, policy)
	if err := store.Save(ctx, id, state); err != nil {
		t.Fatal(err)
	}
	check("PROXY_AUTH_FAILED", "degraded", 9)
	// SQL boundary also rejects unsafe caller/legacy text.
	state.LastError = "https://USER:PASSWORD@proxy.invalid/?token=SECRET"
	if err := store.Save(ctx, id, state); err != nil {
		t.Fatal(err)
	}
	check("PROXY_HEALTH_FAILED", "degraded", 9)
	state = state.Apply(HealthResult{Status: StatusHealthy, CheckedAt: checked.Add(time.Second), Latency: 2 * time.Millisecond}, policy)
	if err := store.Save(ctx, id, state); err != nil {
		t.Fatal(err)
	}
	check("", "degraded", 2)
	state = state.Apply(HealthResult{Status: StatusHealthy, CheckedAt: checked.Add(2 * time.Second), Latency: 3 * time.Millisecond}, policy)
	if err := store.Save(ctx, id, state); err != nil {
		t.Fatal(err)
	}
	check("", "healthy", 3)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.Save(canceled, id, HealthState{Status: StatusDown, LastError: "PROXY_AUTH_FAILED"}); !errors.Is(err, ErrHealthStore) {
		t.Fatalf("canceled write not reported: %v", err)
	}
	check("", "healthy", 3)
	if err := store.Save(ctx, "00000000-0000-0000-0000-000000000000", state); !errors.Is(err, ErrHealthStore) {
		t.Fatalf("missing row not reported: %v", err)
	}
}
