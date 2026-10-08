package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func seedPurgeDependencies(t *testing.T, db *sql.DB) (string, string, string) {
	t.Helper()
	a, d, p := seedPanel(t, db, "http://purge.test")
	sqlMust(t, db, "UPDATE accounts SET enabled=false,provider_state='LOCKED',runtime_status='DELETE_PENDING',deletion_requested_at=now()-interval '3 minutes' WHERE id=$1", a)
	sqlMust(t, db, "UPDATE droplets SET state='RETIRING' WHERE id=$1", d)
	sqlMust(t, db, "INSERT INTO account_deletion_jobs(account_id) VALUES($1)", a)
	sqlMust(t, db, "INSERT INTO secrets(id,account_id,kind,ciphertext,nonce,key_version) VALUES('purge-secret',$1,'api_token','test','test',1)", a)
	sqlMust(t, db, "INSERT INTO account_billing_snapshots(account_id,data) VALUES($1,'{}')", a)
	var g, j string
	if err := db.QueryRow("INSERT INTO bulk_user_generations(panel_id,inbound_id,purpose,marker) VALUES($1,1,'POLICY',$1::uuid::text) RETURNING id::text", p).Scan(&g); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,idempotency_key,state) VALUES($1,$2,1,'test-client','CREATE','purge-job','SUCCEEDED') RETURNING id::text", a, p).Scan(&j); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "INSERT INTO bulk_user_ownership(generation_id,client_id,email,state,mutation_job_id) VALUES($1,'test-client','test','ACTIVE',$2)", g, j)
	sqlMust(t, db, "INSERT INTO bulk_scale_runs(generation_id,target_users,baseline,phase,expires_at) VALUES($1,2,'{}','SUCCEEDED',now())", g)
	sqlMust(t, db, "INSERT INTO bulk_lifecycle_scopes(panel_id,inbound_id,generation_id,expires_at) VALUES($1,1,$2,now())", p, g)
	sqlMust(t, db, "UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,panel_id=$1,inbound_id=1", p)
	sqlMust(t, db, "UPDATE bulk_user_shrink_gate SET enabled=true,panel_id=$1,inbound_id=1", p)
	sqlMust(t, db, "UPDATE bulk_client_execution_gate SET enabled=true,kill_switch=false,panel_id=$1,inbound_id=1,expires_at=now()+interval '1 hour'", p)
	sqlMust(t, db, "UPDATE residential_routing_control SET panel_ids=ARRAY[$1::uuid]", p)

	sqlMust(t, db, "INSERT INTO output_config_snapshots(panel_id,uri,client_id,last_seen_at,visible_until) VALUES($1,'vless://test','test-client',now(),now()+interval '1 hour')", p)
	sqlMust(t, db, "INSERT INTO audit_events(actor,action,resource_type,resource_id,result) VALUES('admin','test','account',$1,'ok')", a)
	return a, d, p
}

func TestExplicitLocalAccountPurgeCascadesAndClosesScopedGates(t *testing.T) {
	db := adminTestDB(t)
	a, _, _ := seedPurgeDependencies(t, db)
	other, _, _ := seedPanel(t, db, "http://other.test")
	sqlMust(t, db, "INSERT INTO audit_events(account_id,actor,action,resource_type,resource_id,result) VALUES($1,'admin','test','account',$2,'ok')", other, a)
	sqlMust(t, db, "UPDATE server_protection_control SET enabled=true,scope='selected',panel_ids=ARRAY(SELECT id FROM panel_instances)")
	sqlMust(t, db, "UPDATE proxy_economy_policy SET enabled=true,canary_account_id=$1", a)
	for _, owner := range []string{a, "replace-" + a, other} {
		sqlMust(t, db, "INSERT INTO proxy_traffic_hourly(hour,process_id,role,owner_id,proxy_id,purpose,tx_bytes,rx_bytes,connections,requests,errors) VALUES(date_trunc('hour',now()),'purge-fixture','account',$1,'p','identity',7,0,0,0,0)", owner)
	}
	sqlMust(t, db, "INSERT INTO proxy_traffic_hourly(hour,process_id,role,owner_id,proxy_id,purpose,tx_bytes,rx_bytes,connections,requests,errors) VALUES(date_trunc('hour',now()),'purge-fixture','base_proxy',$1,'p','identity',8,0,0,0,0)", a)
	ctx := context.Background()
	// A failed step must roll back every deletion and gate change.
	sqlMust(t, db, "CREATE FUNCTION deny_purge() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fault'; END $$")
	sqlMust(t, db, "CREATE TRIGGER injected_purge_fault BEFORE DELETE ON accounts FOR EACH ROW EXECUTE FUNCTION deny_purge()")
	if err := app.PurgeSavedAccount(ctx, db, a, "LOCKED", 1); err == nil {
		t.Fatal("fault hidden")
	}
	var n int
	var open bool
	if err := db.QueryRow("SELECT count(*) FROM secrets WHERE account_id=$1", a).Scan(&n); err != nil || n != 1 {
		t.Fatal("partial purge", n, err)
	}
	if err := db.QueryRow("SELECT enabled FROM client_mutation_execution_gate").Scan(&open); err != nil || !open {
		t.Fatal("partial gate change", err)
	}
	sqlMust(t, db, "DROP TRIGGER injected_purge_fault ON accounts")
	if err := app.PurgeSavedAccount(ctx, db, a, "LOCKED", 1); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"secrets", "account_deletion_jobs", "account_billing_snapshots", "droplets", "deployments", "panel_instances", "client_mutation_jobs"} {
		if err := db.QueryRow("SELECT count(*) FROM "+table+" WHERE account_id=$1", a).Scan(&n); err != nil || n != 0 {
			t.Fatal(table, n, err)
		}
	}
	for _, table := range []string{"bulk_user_ownership", "bulk_lifecycle_scopes", "bulk_scale_runs", "output_config_snapshots"} {
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal(table, n, err)
		}
	}
	if err := db.QueryRow("SELECT enabled OR NOT kill_switch FROM client_mutation_execution_gate").Scan(&open); err != nil || open {
		t.Fatal("gate widened", err)
	}
	if err := db.QueryRow("SELECT enabled FROM bulk_user_shrink_gate").Scan(&open); err != nil || open {
		t.Fatal("shrink gate widened", err)
	}
	if err := db.QueryRow("SELECT count(*) FROM panel_instances WHERE account_id=$1", other).Scan(&n); err != nil || n != 1 {
		t.Fatal("other account changed", n, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM audit_events WHERE account_id=$1", other).Scan(&n); err != nil || n != 1 {
		t.Fatal("unrelated audit removed", n, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM bulk_client_execution_gate WHERE NOT enabled AND kill_switch AND panel_id IS NULL AND inbound_id IS NULL").Scan(&n); err != nil || n != 1 {
		t.Fatal("bulk gate missing or widened", n, err)
	}
	if err := db.QueryRow("SELECT cardinality(panel_ids) FROM residential_routing_control").Scan(&n); err != nil || n != 0 {
		t.Fatal("deleted panel remained in routing scope", n, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM proxy_traffic_hourly WHERE role='account' AND (owner_id=$1 OR owner_id='replace-'||$1)", a).Scan(&n); err != nil || n != 0 {
		t.Fatal("account telemetry retained", n, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM proxy_traffic_hourly").Scan(&n); err != nil || n != 2 {
		t.Fatal("unrelated telemetry deleted", n, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM server_protection_control WHERE enabled AND scope='selected' AND panel_ids=ARRAY(SELECT id FROM panel_instances WHERE account_id=$1)", other).Scan(&n); err != nil || n != 1 {
		t.Fatal("protection scope changed", n, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM proxy_economy_policy WHERE NOT enabled AND canary_account_id IS NULL").Scan(&n); err != nil || n != 1 {
		t.Fatal("canary widened", n, err)
	}
	if err := app.PurgeSavedAccount(ctx, db, a, "LOCKED", 1); err != nil {
		t.Fatal("lost response replay", err)
	}
}

func TestLocalAccountPurgeRejectsStaleUnsafeAndRacingRequests(t *testing.T) {
	db := adminTestDB(t)
	a, _, _ := seedPurgeDependencies(t, db)
	ctx := context.Background()
	for _, tc := range []struct {
		state string
		n     int
	}{{"TOKEN_INVALID", 1}, {"LOCKED", 0}} {
		if err := app.PurgeSavedAccount(ctx, db, a, tc.state, tc.n); !errors.Is(err, app.ErrAccountPurgeConflict) {
			t.Fatal(err)
		}
	}
	sqlMust(t, db, "UPDATE accounts SET enabled=true WHERE id=$1", a)
	if err := app.PurgeSavedAccount(ctx, db, a, "LOCKED", 1); !errors.Is(err, app.ErrAccountPurgeConflict) {
		t.Fatal(err)
	}
	sqlMust(t, db, "UPDATE accounts SET enabled=false WHERE id=$1", a)
	// A provider mutation already in flight must settle before local deletion.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("SELECT pg_advisory_xact_lock_shared(hashtextextended($1,0))", "account-mutation:"+a); err != nil {
		t.Fatal(err)
	}
	c, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	if err = app.PurgeSavedAccount(c, db, a, "LOCKED", 1); err == nil {
		t.Fatal("purge raced provider mutation")
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", a).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
}

func TestLocalPurgeAPIRequiresExplicitCloudAcknowledgement(t *testing.T) {
	db := adminTestDB(t)
	a, _, _ := seedPurgeDependencies(t, db)
	s := Server{DB: db}
	progress := s.accountDeletionProgress(context.Background(), a).(map[string]any)
	if progress["provider_state"] != "LOCKED" {
		t.Fatal("deletion loses raw provider blocker", progress)
	}
	call := func(ack bool, role auth.Role) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(map[string]any{"account_id": a, "expected_provider_state": "LOCKED", "expected_remaining_servers": 1, "acknowledge_cloud_resources_unverified": ack})
		req := httptest.NewRequest("POST", "/api/v1/accounts/"+a+"/purge", strings.NewReader(string(raw)))
		req.SetPathValue("id", a)
		req = req.WithContext(context.WithValue(req.Context(), principalKey{}, auth.Principal{Role: role}))
		w := httptest.NewRecorder()
		s.purgeAccount(w, req)
		return w
	}
	if w := call(false, auth.Admin); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	// A live provider mutation must return an actionable bounded conflict, with
	// every saved row retained. It cannot be silently reported as success.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("SELECT pg_advisory_xact_lock_shared(hashtextextended($1,0))", "account-mutation:"+a); err != nil {
		t.Fatal(err)
	}
	if w := call(true, auth.Admin); w.Code != 409 || !strings.Contains(w.Body.String(), "account_cleanup_busy") || w.Header().Get("Retry-After") == "" {
		t.Fatal(w.Code, w.Body.String())
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if w := call(true, auth.Admin); w.Code != 200 || !strings.Contains(w.Body.String(), "\"provider_deletion_verified\":false") {
		t.Fatal(w.Code, w.Body.String())
	}
}
