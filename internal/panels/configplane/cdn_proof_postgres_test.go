package configplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflare"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Each regression gets a schema in the explicitly disposable release database.
func cdnProofDB(t *testing.T) *sql.DB {
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
	schema := "cdn_" + strings.ReplaceAll(id, "-", "")
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
	db.SetMaxOpenConns(6)
	t.Cleanup(func() { db.Close(); admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	dir := t.TempDir()
	files, err := filepath.Glob("../../../migrations/*.up.sql")
	if err != nil || len(files) == 0 {
		t.Fatal("migration files unavailable", err)
	}
	for _, file := range files {
		b, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		b = []byte(strings.ReplaceAll(string(b), "tc.table_schema='public'", "tc.table_schema=current_schema()"))
		if e = os.WriteFile(filepath.Join(dir, filepath.Base(file)), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = (migrate.Runner{DB: db, Dir: dir}).Up(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}
func cdnExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, e := db.Exec(q, args...); e != nil {
		t.Fatal(e)
	}
}
func cdnFixture(t *testing.T, db *sql.DB) (string, string, string) {
	t.Helper()
	owner, _ := sanaei.UUIDv4()
	drop, _ := sanaei.UUIDv4()
	panel, _ := sanaei.UUIDv4()
	account, _ := sanaei.UUIDv4()
	profile, _ := sanaei.UUIDv4()
	cdnExec(t, db, "INSERT INTO accounts(id,name,provider,secret_ref,provider_state) VALUES($1,'cdn fixture','digitalocean','fixture','ACTIVE')", owner)
	cdnExec(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state,expires_at) VALUES($1,$2,'fixture','READY',now()+interval '2 hours')", drop, owner)
	cdnExec(t, db, "INSERT INTO panel_instances(id,account_id,droplet_id,driver,base_url,auth_secret_ref) VALUES($1,$2,$3,'sanaei-3x-ui','http://fixture.invalid','fixture')", panel, owner, drop)
	cdnExec(t, db, "INSERT INTO cloudflare_accounts(id,name) VALUES($1,'fixture')", account)
	cdnExec(t, db, "INSERT INTO cloudflare_zones(account_id,id,name,status,enabled,ssl_strict,checked_at) VALUES($1,'zone','example.com','active',true,true,now())", account)
	raw, _ := json.Marshal(map[string]any{"cloudflare_account": account, "cloudflare_operator": ""})
	cdnExec(t, db, "INSERT INTO config_driver_profiles(id,name,spec,enabled,fleet,classes) VALUES($1,'cdn',$2,true,true,ARRAY['DIRECT','RESIDENTIAL'])", profile, raw)
	cdnExec(t, db, "INSERT INTO config_driver_listeners(profile_id,panel_id,managed_key,credential_ref,applied_spec,applied_revision,mutation_state,state,verified_at) VALUES($1,$2,$3,'fixture',$4,1,'CONFIRMED','VERIFIED',now())", profile, panel, "dob:profile:"+profile, raw)
	return account, profile, panel
}
func cdnRecord(t *testing.T, db *sql.DB, a, p, panel string, at time.Time, classes string) string {
	t.Helper()
	id, _ := sanaei.UUIDv4()
	cdnExec(t, db, "INSERT INTO cloudflare_records(id,account_id,zone_id,profile_id,panel_id,hostname,origin_ip,state,expires_at,observed_at,verified_at,verified_revision,verified_classes) VALUES($1,$2,'zone',$3,$4,$5,'192.0.2.5','DNS_READY',now()+interval '1 hour',now(),$6,1,$7)", id, a, p, panel, id+".example.com", at, classes)
	return id
}
func TestCDNClassProofsPreserveIndependentEndpointTimes(t *testing.T) {
	db := cdnProofDB(t)
	a, p, panel := cdnFixture(t, db)
	ctx := context.Background()
	old := time.Now().UTC().Add(-4 * time.Minute).Truncate(time.Microsecond)
	recent := time.Now().UTC().Truncate(time.Microsecond)
	sibling := cdnRecord(t, db, a, p, panel, old, "{DIRECT,RESIDENTIAL}")
	latest := cdnRecord(t, db, a, p, panel, recent, "{DIRECT}")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	classes, err := cloudflare.FreshEndpointClasses(ctx, tx, a, p, panel, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !classes["DIRECT"].Equal(recent) || !classes["RESIDENTIAL"].Equal(old) {
		t.Fatal("sibling class revoked or timestamp extended", classes)
	}
	if err = persistClassProofs(ctx, tx, p, panel, []string{"DIRECT", "RESIDENTIAL"}, classes); err != nil {
		t.Fatal(err)
	}
	var at time.Time
	var code string
	if err = tx.QueryRow("SELECT verified_at,error_code FROM config_driver_class_proofs WHERE profile_id=$1 AND panel_id=$2 AND route_class='RESIDENTIAL'", p, panel).Scan(&at, &code); err != nil || !at.Equal(old) || code != "" {
		t.Fatal("class proof lost original evidence", at, code, err)
	}
	// A stale revision must not revoke the current endpoint.
	if err = cloudflare.RejectEndpoint(ctx, tx, latest, p, panel, 2); err != nil {
		t.Fatal(err)
	}
	classes, err = cloudflare.FreshEndpointClasses(ctx, tx, a, p, panel, 1)
	if err != nil || !classes["DIRECT"].Equal(recent) {
		t.Fatal("obsolete rejection changed evidence", err)
	}
	if err = cloudflare.RejectEndpoint(ctx, tx, latest, p, panel, 1); err != nil {
		t.Fatal(err)
	}
	classes, err = cloudflare.FreshEndpointClasses(ctx, tx, a, p, panel, 1)
	if err != nil || !classes["DIRECT"].Equal(old) || !classes["RESIDENTIAL"].Equal(old) {
		t.Fatal("rejection revoked healthy sibling", classes, err)
	}
	if _, err = tx.Exec("UPDATE cloudflare_records SET verified_at=now()-interval '6 minutes' WHERE id=$1", sibling); err != nil {
		t.Fatal(err)
	}
	classes, err = cloudflare.FreshEndpointClasses(ctx, tx, a, p, panel, 1)
	if err != nil || len(classes) != 0 {
		t.Fatal("expired proof survived", classes, err)
	}
	if err = persistClassProofs(ctx, tx, p, panel, []string{"DIRECT", "RESIDENTIAL"}, classes); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = tx.QueryRow("SELECT count(*) FROM config_driver_class_proofs WHERE profile_id=$1 AND (verified_at IS NOT NULL OR error_code='')", p).Scan(&count); err != nil || count != 0 {
		t.Fatal("expired class remained published", count, err)
	}
}
func TestCDNProofFencesAndStructuralFailure(t *testing.T) {
	db := cdnProofDB(t)
	a, p, panel := cdnFixture(t, db)
	ctx := context.Background()
	id := cdnRecord(t, db, a, p, panel, time.Now().Add(-4*time.Minute), "{DIRECT,RESIDENTIAL}")
	s := Service{DB: db}
	s.batchFailure(ctx, p, panel, "profile_runtime_traffic_unverified", false)
	var state string
	if e := db.QueryRow("SELECT state FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", p, panel).Scan(&state); e != nil || state != "VERIFIED" {
		t.Fatal("fresh sibling lost", state, e)
	}
	// Native structural failure is stronger than endpoint evidence.
	s.batchFailure(ctx, p, panel, "profile_runtime_users_unverified", false)
	if e := db.QueryRow("SELECT state FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", p, panel).Scan(&state); e != nil || state != "BLOCKED" {
		t.Fatal("native failure hidden by DNS evidence", state, e)
	}
	cases := map[string]string{
		"stale_native_proof": "UPDATE cloudflare_records SET verified_at=now()-interval '6 minutes'",
		"near_expiry":        "UPDATE cloudflare_records SET expires_at=now()+interval '4 minutes'",
		"stale_observation":  "UPDATE cloudflare_records SET observed_at=now()-interval '11 minutes'",
		"wrong_revision":     "UPDATE cloudflare_records SET verified_revision=2",
		"wrong_operator":     "UPDATE cloudflare_records SET operator='mci'",
		"not_dns_ready":      "UPDATE cloudflare_records SET state='PREPARED'",
		"zone_disabled":      "UPDATE cloudflare_zones SET enabled=false",
		"tls_not_strict":     "UPDATE cloudflare_zones SET ssl_strict=false",
		"zone_missing":       "UPDATE cloudflare_zones SET present=false",
		"stale_zone":         "UPDATE cloudflare_zones SET checked_at=now()-interval '11 minutes'",
		"account_disabled":   "UPDATE cloudflare_accounts SET enabled=false",
		"profile_disabled":   "UPDATE config_driver_profiles SET enabled=false",
		"profile_revised":    "UPDATE config_driver_profiles SET revision=2",
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			tx, e := db.BeginTx(ctx, nil)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback()
			if _, e = tx.Exec(change); e != nil {
				t.Fatal(e)
			}
			classes, e := cloudflare.FreshEndpointClasses(ctx, tx, a, p, panel, 1)
			if e != nil || len(classes) != 0 {
				t.Fatal("ineligible endpoint retained", id, classes, e)
			}
		})
	}
	// Failed class/candidate cancellation cannot mutate confirmed endpoint proof.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	e = cloudflare.RejectEndpoint(cancelled, tx, id, p, panel, 1)
	_ = tx.Rollback()
	if e == nil {
		t.Fatal("cancelled rejection succeeded")
	}
	var retained bool
	if e = db.QueryRow("SELECT verified_at IS NOT NULL FROM cloudflare_records WHERE id=$1", id).Scan(&retained); e != nil || !retained {
		t.Fatal("cancelled work revoked proof", e)
	}
}
