package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// The release gate supplies a fresh socket-only cluster. Never accept a normal
// application's DATABASE_URL for a destructive schema fixture.
func checksumFixtureDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "bulk_test") {
		t.Fatal("isolated bulk_test database required")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var suffix string
	if err := admin.QueryRowContext(ctx, "SELECT replace(gen_random_uuid()::text,'-','')").Scan(&suffix); err != nil {
		t.Fatal(err)
	}
	schema := "migration_checksum_" + suffix
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.ExecContext(c, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("fixture cleanup: %v", err)
		}
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMigrationLegacyChecksumBackfillAndDrift(t *testing.T) {
	db := checksumFixtureDB(t)
	for _, initial := range []struct {
		name  string
		value any
	}{{"null", nil}, {"empty", ""}} {
		t.Run(initial.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			mustExec := func(query string, args ...any) {
				t.Helper()
				if _, err := db.ExecContext(ctx, query, args...); err != nil {
					t.Fatal(err)
				}
			}
			mustExec("DROP TABLE IF EXISTS schema_migrations, checksum_fixture")
			mustExec("CREATE TABLE schema_migrations(version text PRIMARY KEY, checksum text, applied_at timestamptz NOT NULL DEFAULT now())")
			mustExec("CREATE TABLE checksum_fixture(value text NOT NULL)")
			mustExec("INSERT INTO checksum_fixture VALUES('preserved')")
			mustExec("INSERT INTO schema_migrations(version,checksum) VALUES('001_legacy',$1)", initial.value)
			dir := t.TempDir()
			path := filepath.Join(dir, "001_legacy.up.sql")
			// Replaying this already-applied migration would destroy the fixture.
			raw := []byte("DELETE FROM checksum_fixture;")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			runner := Runner{DB: db, Dir: dir, PreserveData: true}
			if err := runner.Up(ctx); err != nil {
				t.Fatal(err)
			}
			var got, retained string
			if err := db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version='001_legacy'").Scan(&got); err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf("%x", sha256.Sum256(raw))
			if got != want {
				t.Fatalf("legacy %s checksum was not sealed: got %q want %q", initial.name, got, want)
			}
			if err := db.QueryRowContext(ctx, "SELECT value FROM checksum_fixture").Scan(&retained); err != nil || retained != "preserved" {
				t.Fatalf("already-applied migration replayed: value=%q err=%v", retained, err)
			}
			if err := runner.Up(ctx); err != nil {
				t.Fatalf("unchanged migration is not idempotent: %v", err)
			}
			if err := os.WriteFile(path, []byte("DELETE FROM checksum_fixture; -- changed"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := runner.Up(ctx); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
				t.Fatalf("changed legacy migration was accepted: %v", err)
			}
			if err := db.QueryRowContext(ctx, "SELECT value FROM checksum_fixture").Scan(&retained); err != nil || retained != "preserved" {
				t.Fatalf("checksum failure changed retained data: value=%q err=%v", retained, err)
			}
			if err := db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version='001_legacy'").Scan(&got); err != nil || got != want {
				t.Fatalf("checksum mismatch rewrote history: checksum=%q err=%v", got, err)
			}
		})
	}
}
