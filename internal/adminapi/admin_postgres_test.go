package adminapi

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func adminTestDB(t *testing.T) *sql.DB {
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
	files, e := filepath.Glob("../../migrations/*.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	for _, file := range files {
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
func TestProxyDeleteSharedResidentialAndFailClosedDetach(t *testing.T) {
	db := adminTestDB(t)
	const proxy = "11111111-1111-4111-8111-111111111111"
	const account = "22222222-2222-4222-8222-222222222222"
	sqlMust(t, db, `INSERT INTO proxies(id,name,type,host,port) VALUES($1,'test','socks5','localhost',1080)`, proxy)
	sqlMust(t, db, `INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'test','digitalocean','test')`, account)
	sqlMust(t, db, `INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,'proxy_required',$2)`, account, proxy)
	sqlMust(t, db, `INSERT INTO residential_proxies(proxy_id,outbound_tag,priority,enabled) VALUES($1,'residential-ads-test',1,true)`, proxy)
	s := Server{DB: db}
	kept, err := s.removeProxy(context.Background(), proxy, true)
	if err != nil || !kept {
		t.Fatal(kept, err)
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM residential_proxies WHERE proxy_id=$1", proxy).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err = db.QueryRow("SELECT count(*) FROM network_profiles WHERE proxy_id=$1", proxy).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if _, err = s.removeProxy(context.Background(), proxy, false); err != nil {
		t.Fatal(err)
	}
	var mode string
	var p sql.NullString
	if err = db.QueryRow("SELECT mode,proxy_id::text FROM network_profiles WHERE account_id=$1", account).Scan(&mode, &p); err != nil || mode != "proxy_required" || p.Valid {
		t.Fatal(mode, p, err)
	}
	if _, err = s.removeProxy(context.Background(), proxy, false); err != nil {
		t.Fatal("idempotent delete", err)
	}
}
func TestProxyDeleteWaitsForInFlightAccountRequest(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	const proxy = "11111111-1111-4111-8111-111111111111"
	const account = "22222222-2222-4222-8222-222222222222"
	sqlMust(t, db, `INSERT INTO proxies(id,name,type,host,port) VALUES($1,'test','socks5','localhost',1080)`, proxy)
	sqlMust(t, db, `INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'test','digitalocean','test')`, account)
	sqlMust(t, db, `INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,'proxy_required',$2)`, account, proxy)
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "SELECT pg_advisory_lock_shared(hashtextextended($1,0))", "account-route:"+account); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(ctx, "SELECT pg_advisory_unlock_shared(hashtextextended($1,0))", "account-route:"+account)
	done := make(chan error, 1)
	go func() { _, err := (&Server{DB: db}).removeProxy(ctx, proxy, false); done <- err }()
	select {
	case err := <-done:
		t.Fatal("deleted while request in flight", err)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err = conn.ExecContext(ctx, "SELECT pg_advisory_unlock_shared(hashtextextended($1,0))", "account-route:"+account); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("delete did not resume")
	}
}
func TestDashboardSQLUsesCompleteSchema(t *testing.T) {
	db := adminTestDB(t)
	var raw []byte
	if err := db.QueryRow(dashboardCountsSQL).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"active_servers" : 0`) {
		t.Fatal(string(raw))
	}
}
