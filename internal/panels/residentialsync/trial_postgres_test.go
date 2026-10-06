package residentialsync

import (
	"context"
	"database/sql"
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

func trialTestDB(t *testing.T) *sql.DB { return trialTestDBThrough(t, 999999) }
func trialTestDBThrough(t *testing.T, maxVersion int) *sql.DB {
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
func sqlMustTrial(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func TestTrialCompatibleResidentialSelectionPostgres(t *testing.T) {
	db := trialTestDB(t)
	ctx := context.Background()
	sqlMustTrial(t, db, "INSERT INTO global_config_policies(policy_key,ports) VALUES('reality','[443]')")
	sqlMustTrial(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	account, droplet, profile, dep, panel := trialUUID(), trialUUID(), trialUUID(), trialUUID(), trialUUID()
	sqlMustTrial(t, db, "INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'trial','upcloud','test')", account)
	sqlMustTrial(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES($1,$2,'trial-resource','READY')", droplet, account)
	sqlMustTrial(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$2,'trial','{}')", profile, account)
	sqlMustTrial(t, db, `INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step,profile_snapshot) VALUES($1,$2,$3,$4,'READY','done','{"upcloud_trial_compatible":true}')`, dep, account, profile, droplet)
	sqlMustTrial(t, db, "INSERT INTO panel_instances(id,account_id,droplet_id,driver,base_url,auth_secret_ref) VALUES($1,$2,$3,'sanaei-3x-ui','http://trial.test','test')", panel, account, droplet)
	for _, port := range []int{10000, 3106, 443} {
		id := trialUUID()
		sqlMustTrial(t, db, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,outbound_tag,status,last_success_at) VALUES($1,$1::uuid::text,'socks5','proxy.test',$2,$1::uuid::text,'healthy',now()+interval '1 second')", id, port)
	}
	service := Service{DB: db}
	p, _, e := service.policy(ctx, panel)
	if e != nil || len(p.Proxies) != 1 || p.Proxies[0].Port != 443 || p.HealthyCount != 1 {
		t.Fatalf("trial chose prohibited endpoints: %+v %v", p, e)
	}
	sqlMustTrial(t, db, "UPDATE residential_proxies SET enabled=false WHERE port=443")
	p, _, e = service.policy(ctx, panel)
	if e != nil || len(p.Proxies) != 0 || p.HealthyCount != 0 || p.Configured != 3 {
		t.Fatalf("incompatible endpoints were healthy: %+v %v", p, e)
	}
	// A later standard deployment must not inherit an old trial row.
	next := trialUUID()
	sqlMustTrial(t, db, "UPDATE deployments SET state='FAILED',created_at=now()-interval '1 minute' WHERE id=$1", dep)
	sqlMustTrial(t, db, "INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step,profile_snapshot) VALUES($1,$2,$3,$4,'READY','done','{}')", next, account, profile, droplet)
	p, _, e = service.policy(ctx, panel)
	if e != nil || len(p.Proxies) < 1 || p.HealthyCount != 2 {
		t.Fatalf("standard deployment changed: %+v %v", p, e)
	}
}
func trialUUID() string { id, _ := sanaei.UUIDv4(); return id }
