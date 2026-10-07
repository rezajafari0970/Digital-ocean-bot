package usercapacity

import (
	"context"
	"database/sql"
	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func admissionTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "bulk_test") {
		t.Fatal("test database must contain bulk_test")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := sanaei.UUIDv4()
	schema := "bulk_" + strings.ReplaceAll(id, "-", "")
	if _, err = admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
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
	t.Cleanup(func() { db.Close(); admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); admin.Close() })
	fixture := `CREATE TABLE accounts(id uuid PRIMARY KEY,enabled boolean,provider_state text);
 CREATE TABLE droplets(id uuid PRIMARY KEY,state text,expires_at timestamptz);
 CREATE TABLE deployments(droplet_id uuid,state text);
 CREATE TABLE panel_instances(id uuid PRIMARY KEY,account_id uuid,droplet_id uuid,enabled boolean);
 CREATE TABLE panel_inbound_inventory(panel_id uuid,remote_id bigint,present boolean,enabled boolean,port integer DEFAULT 443);`
	if _, err = db.Exec(fixture); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000124_client_mutation_jobs", "000125_bulk_user_ownership", "000127_client_mutation_execution_gate", "000131_bulk_user_unknown_outcome_recovery", "000134_durable_bulk_create", "000135_bulk_scale_recovery", "000136_bulk_lifecycle", "000137_bulk_lifecycle_admission", "000161_client_mutation_panel_health"} {
		b, e := os.ReadFile(filepath.Join("../../../migrations", name+".up.sql"))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(b)); e != nil {
			t.Fatal(name, e)
		}
	}
	_, err = db.Exec(`INSERT INTO accounts VALUES('11111111-1111-4111-8111-111111111111',true,'ACTIVE');
 INSERT INTO droplets(id,state) VALUES('22222222-2222-4222-8222-222222222222','READY');
 INSERT INTO deployments VALUES('22222222-2222-4222-8222-222222222222','PANEL_COMPLETE');
 INSERT INTO panel_instances VALUES('55555555-5555-4555-8555-555555555555','11111111-1111-4111-8111-111111111111','22222222-2222-4222-8222-222222222222',true);
 INSERT INTO panel_inbound_inventory(panel_id,remote_id,present,enabled) VALUES('55555555-5555-4555-8555-555555555555',1,true,true);
 INSERT INTO bulk_user_generations(id,panel_id,inbound_id,purpose,marker) VALUES('44444444-4444-4444-8444-444444444444','55555555-5555-4555-8555-555555555555',1,'CANARY','test');
 UPDATE bulk_client_execution_gate SET enabled=true,kill_switch=false,panel_id='55555555-5555-4555-8555-555555555555',inbound_id=1,remaining_batches=1,expires_at=now()+interval '5 minutes';`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

const admissionPanel = "55555555-5555-4555-8555-555555555555"

func TestAdmissionPostgresGuardsConcurrencyAndNoRearm(t *testing.T) {
	db := admissionTestDB(t)
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TABLE global_config_policies(policy_key text PRIMARY KEY,enabled bool,ports jsonb);
 INSERT INTO global_config_policies VALUES('reality',true,'[443]');
 UPDATE bulk_lifecycle_control SET enabled=true,auto_enroll=true,max_active_scopes=1,operation_budget=23,max_batch_size=10;
 UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,panel_id=NULL,inbound_id=NULL;
 UPDATE droplets SET expires_at=now()+interval '2 hours';`)
	s := Service{DB: db}
	admit := func(in int64, port int) {
		t.Helper()
		if err := s.admitLifecycleScope(context.Background(), admissionPanel, in, port); err != nil {
			t.Fatal(err)
		}
	}
	count := func(want int) {
		t.Helper()
		var n int
		if err := db.QueryRow("SELECT count(*) FROM bulk_lifecycle_scopes").Scan(&n); err != nil || n != want {
			t.Fatalf("scopes=%d want=%d err=%v", n, want, err)
		}
	}
	for _, tc := range []struct{ before, after string }{
		{"INSERT INTO client_mutation_panel_health(panel_id,state,failures,retry_after,reason_code) VALUES('55555555-5555-4555-8555-555555555555','COOLDOWN',1,now()+interval '1 minute','RUNTIME_UNAVAILABLE')", "DELETE FROM client_mutation_panel_health"},
		{"INSERT INTO client_mutation_panel_health(panel_id,state,failures,reason_code) VALUES('55555555-5555-4555-8555-555555555555','QUARANTINED',1,'VERIFICATION_FAILED')", "DELETE FROM client_mutation_panel_health"},
		{"UPDATE bulk_lifecycle_control SET auto_enroll=false", "UPDATE bulk_lifecycle_control SET auto_enroll=true"},
		{"UPDATE client_mutation_execution_gate SET kill_switch=true", "UPDATE client_mutation_execution_gate SET kill_switch=false"},
		{"UPDATE client_mutation_execution_gate SET panel_id='55555555-5555-4555-8555-555555555555',inbound_id=2", "UPDATE client_mutation_execution_gate SET panel_id=NULL,inbound_id=NULL"},
		{"UPDATE global_config_policies SET enabled=false", "UPDATE global_config_policies SET enabled=true"},
		{"UPDATE accounts SET provider_state='LOCKED'", "UPDATE accounts SET provider_state='ACTIVE'"},
		{"UPDATE droplets SET expires_at=now()+interval '30 seconds'", "UPDATE droplets SET expires_at=now()+interval '2 hours'"},
	} {
		exec(tc.before)
		admit(1, 443)
		count(0)
		exec(tc.after)
	}
	admit(1, 8443)
	count(0)
	exec(`INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,state,idempotency_key,payload) VALUES('11111111-1111-4111-8111-111111111111','55555555-5555-4555-8555-555555555555',1,'historical','CREATE','FAILED','history','{}');`)
	admit(1, 443)
	count(0)
	exec("DELETE FROM client_mutation_jobs") // isolated test schema only
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.admitLifecycleScope(context.Background(), admissionPanel, 1, 443) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	count(1)
	var n, chunk int
	var bounded bool
	if err := db.QueryRow(`SELECT remaining_operations,max_batch_size,s.expires_at<=d.expires_at-interval '10 seconds' FROM bulk_lifecycle_scopes s JOIN panel_instances p ON p.id=s.panel_id JOIN droplets d ON d.id=p.droplet_id`).Scan(&n, &chunk, &bounded); err != nil || n != 23 || chunk != 10 || !bounded {
		t.Fatalf("budget=%d chunk=%d bounded=%v err=%v", n, chunk, bounded, err)
	}
	admit(2, 443)
	count(1) // global cap
	exec("UPDATE bulk_lifecycle_scopes SET enabled=false,remaining_operations=0")
	admit(1, 443)
	if err := db.QueryRow("SELECT remaining_operations FROM bulk_lifecycle_scopes WHERE inbound_id=1").Scan(&n); err != nil || n != 0 {
		t.Fatalf("rearmed: %d %v", n, err)
	}
	// A gate closure committed while admission is waiting on a row lock must
	// be visible to its post-lock eligibility query.
	exec("UPDATE bulk_lifecycle_control SET max_active_scopes=2")
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec("UPDATE client_mutation_execution_gate SET kill_switch=true"); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.admitLifecycleScope(context.Background(), admissionPanel, 2, 443) }()
	select {
	case err := <-done:
		tx.Rollback()
		t.Fatalf("admission did not wait for authorization lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	count(1)
}
