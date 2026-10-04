package panelbootstrap

import (
	"context"
	"database/sql"
	_ "github.com/lib/pq"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func repairTestDB(t *testing.T) *sql.DB { return repairTestDBThrough(t, 999999) }
func repairTestDBThrough(t *testing.T, maxVersion int) *sql.DB {
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
	id, _ := sanaei.UUIDv4()
	schema := "admin_" + strings.ReplaceAll(id, "-", "")
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
	db.SetMaxOpenConns(12)
	t.Cleanup(func() { db.Close(); admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// Historical migration 51 hard-codes public only for its catalog lookup.
	// Adapt that lookup to this isolated schema; production migration files stay unchanged.
	dir := t.TempDir()
	files, e := filepath.Glob("../../../migrations/*.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	for _, file := range files {
		version, _ := strconv.Atoi(strings.Split(filepath.Base(file), "_")[0])
		if version > maxVersion {
			continue
		}
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		raw = []byte(strings.ReplaceAll(string(raw), "tc.table_schema='public'", "tc.table_schema=current_schema()"))
		if e = os.WriteFile(filepath.Join(dir, filepath.Base(file)), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if err = (migrate.Runner{DB: db, Dir: dir}).Up(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}
func sqlMust(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}
