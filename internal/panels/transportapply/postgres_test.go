package transportapply

import (
	"context"
	"database/sql"
	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func applyTestDB(t *testing.T) *sql.DB { return applyTestDBThrough(t, 999999) }
func applyTestDBThrough(t *testing.T, maxVersion int) *sql.DB {
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
func seedPanel(t *testing.T, db *sql.DB, base string) (string, string, string) {
	t.Helper()
	a, _ := sanaei.UUIDv4()
	d, _ := sanaei.UUIDv4()
	p, _ := sanaei.UUIDv4()
	sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref,provider_state) VALUES($1,'test','digitalocean','test','ACTIVE')", a)
	sqlMust(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state,expires_at) VALUES($1,$2,$3,'READY',now()+interval '1 hour')", d, a, "test-"+d)
	sqlMust(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$2,'test','{}')", p, a)
	sqlMust(t, db, "INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step) VALUES(gen_random_uuid(),$1,$3,$2,'PANEL_COMPLETE','done')", a, d, p)
	sqlMust(t, db, "INSERT INTO panel_instances(id,account_id,droplet_id,driver,base_url,auth_secret_ref) VALUES($1,$2,$3,'sanaei-3x-ui',$4,'test')", p, a, d, base)
	sqlMust(t, db, "INSERT INTO xui_panel_deployments(id,account_id,droplet_id,username,password_secret_ref,port,web_path,state) VALUES(gen_random_uuid(),$1,$2,'test','test',8080,'/','COMPLETED')", a, d)
	return a, d, p
}
