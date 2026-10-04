package adminapi

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/cleanup"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResidentialDomainsAndEncryptedCredentialsAreIndependent(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	id, _ := sanaei.UUIDv4()
	sqlMust(t, db, "INSERT INTO proxies(id,name,type,host,port,status,last_success_at) VALUES($1,'account','socks5','account.test',1080,'healthy',now())", id)
	sqlMust(t, db, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,outbound_tag,status,last_success_at) VALUES($1,'residential','socks5','residential.test',1081,'residential-test','healthy',now())", id)
	store, err := secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.PutProxy(ctx, id, "proxy-password", "proxy_password", []byte("account-secret")); err != nil {
		t.Fatal(err)
	}
	if err = store.PutResidentialTx(ctx, db, id, "proxy-password", "proxy_password", []byte("residential-secret")); err != nil {
		t.Fatal(err)
	}
	var before int64
	if err = db.QueryRow("SELECT revision FROM residential_routing_control").Scan(&before); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "UPDATE proxies SET status='down',host='changed-account.test' WHERE id=$1", id)
	var after int64
	if err = db.QueryRow("SELECT revision FROM residential_routing_control").Scan(&after); err != nil || after != before {
		t.Fatal("account rule changed residential revision", err)
	}
	got, err := store.GetResidential(ctx, id, "proxy-password")
	if err != nil || string(got) != "residential-secret" {
		t.Fatal("independent credential lookup", err)
	}
	s := Server{DB: db}
	if _, err = s.removeProxy(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetResidential(ctx, id, "proxy-password")
	if err != nil || string(got) != "residential-secret" {
		t.Fatal("ordinary delete damaged residential", err)
	}
	var state string
	if err = db.QueryRow("SELECT status FROM residential_proxies WHERE proxy_id=$1", id).Scan(&state); err != nil || state != "healthy" {
		t.Fatal(state, err)
	}
	if _, err = s.removeProxy(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetResidential(ctx, id, "proxy-password"); err == nil {
		t.Fatal("residential credentials not removed")
	}
}
func TestCleanupCancellationWaitsForMutationAndUnblocksPolicy(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	_, _, panel := seedPanel(t, db, "http://panel.test")
	sqlMust(t, db, "INSERT INTO global_config_policies(policy_key,enabled,ports,target_users_per_inbound,users_per_second) VALUES('reality',true,'[443]',2,1) ON CONFLICT(policy_key) DO UPDATE SET enabled=true")
	store := cleanup.Store{DB: db}
	id, err := store.Start(ctx, []string{panel})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	sqlMust(t, db, "UPDATE panel_cleanup_targets SET state='FAILED',last_error='unavailable' WHERE job_id=$1", id)
	sqlMust(t, db, "UPDATE panel_cleanup_jobs SET state='PAUSED' WHERE id=$1", id)
	if _, err = conn.ExecContext(ctx, "SELECT pg_advisory_lock(628341902731)"); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(ctx, "SELECT pg_advisory_unlock(628341902731)")
	done := make(chan error, 1)
	go func() { done <- store.Cancel(ctx, id) }()
	select {
	case err := <-done:
		t.Fatal("cancel raced in-flight mutation", err)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err = conn.ExecContext(ctx, "SELECT pg_advisory_unlock(628341902731)"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel stuck")
	}
	st, err := store.Current(ctx)
	if err != nil || st.Status != "cancelled" || st.Succeeded != 0 || st.Failed != 1 {
		t.Fatal(st, err)
	}
	// Global save is allowed only after cancellation, not by bypassing its guard.
	s := Server{DB: db}
	req := httptest.NewRequest("PUT", "/", strings.NewReader(`{"route_class":"RESIDENTIAL","profile_revision":0,"enabled":true,"ports":[443],"target_users_per_inbound":2,"users_per_second":1,"sni_selection_mode":"scored","user_quota_expression":"0","user_lifetime_expression":"180"}`))
	w := httptest.NewRecorder()
	s.putGlobalConfig(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err = store.Cancel(ctx, id); err != nil {
		t.Fatal("idempotent cancel", err)
	}
}
func TestCleanupFailureDoesNotStrandOtherTargets(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	_, _, bad := seedPanel(t, db, "http://127.0.0.1:1")
	_, drop, removed := seedPanel(t, db, "http://127.0.0.1:2")
	sqlMust(t, db, "INSERT INTO global_config_policies(policy_key,enabled,ports,target_users_per_inbound,users_per_second) VALUES('reality',true,'[443]',2,1) ON CONFLICT(policy_key) DO UPDATE SET enabled=true")
	store := cleanup.Store{DB: db}
	id, err := store.Start(ctx, []string{bad, removed})
	if err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "UPDATE droplets SET state='DELETED' WHERE id=$1", drop)
	svc := cleanup.Service{DB: db, Runtimes: &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: panelTestSecrets{}, Timeout: time.Millisecond * 100}}}
	for i := 0; i < 3; i++ {
		_ = svc.RunOne(ctx)
	}
	st, err := store.Status(ctx, id)
	if err != nil || st.Status != "paused" || st.Succeeded != 1 || st.Failed != 1 {
		raw, _ := json.Marshal(st)
		t.Fatal(string(raw), err)
	}
}

func TestResidentialUpgradePreservesSecretsAndSharedAccountProxy(t *testing.T) {
	db := adminTestDBThrough(t, 142)
	ctx := context.Background()
	store, err := secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	a, _, _ := seedPanel(t, db, "http://panel.test")
	shared, _ := sanaei.UUIDv4()
	dedicated, _ := sanaei.UUIDv4()
	for _, id := range []string{shared, dedicated} {
		sqlMust(t, db, "INSERT INTO proxies(id,name,type,host,port) VALUES($1,$2,'socks5','example.test',1080)", id, id)
		sqlMust(t, db, "INSERT INTO residential_proxies(proxy_id,outbound_tag) VALUES($1,$2)", id, id)
		if err = store.PutProxy(ctx, id, "proxy-password", "proxy_password", []byte("encrypted-original")); err != nil {
			t.Fatal(err)
		}
		sqlMust(t, db, "UPDATE proxies SET secret_ref='proxy-password' WHERE id=$1", id)
	}
	sqlMust(t, db, "INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,'proxy_required',$2)", a, shared)
	dir := t.TempDir()
	files, _ := filepath.Glob("../../migrations/00014[3-9]*.up.sql")
	for _, f := range files {
		b, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, filepath.Base(f)), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if err = (migrate.Runner{DB: db, Dir: dir}).Up(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{shared, dedicated} {
		b, err := store.GetResidential(ctx, id, "proxy-password")
		if err != nil || string(b) != "encrypted-original" {
			t.Fatal("migration lost encrypted secret", err)
		}
	}
	b, err := store.GetProxy(ctx, shared, "proxy-password")
	if err != nil || string(b) != "encrypted-original" {
		t.Fatal("shared account damaged", err)
	}
	if _, err = store.GetProxy(ctx, dedicated, "proxy-password"); err == nil {
		t.Fatal("residential remained in ordinary proxy domain")
	}
}

func TestResumeAutomationRequiresTerminalCleanupAndPreservesBudgets(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	account, _, panel := seedPanel(t, db, "http://panel.test")
	sqlMust(t, db, "UPDATE accounts SET enabled=true WHERE id=$1", account)
	sqlMust(t, db, "INSERT INTO global_config_policies(policy_key,enabled,ports,target_users_per_inbound,users_per_second) VALUES('reality',true,'[443]',2,1) ON CONFLICT(policy_key) DO UPDATE SET enabled=true")
	sqlMust(t, db, "UPDATE bulk_lifecycle_control SET enabled=true,auto_enroll=true,max_active_scopes=1")
	var generation string
	if err := db.QueryRow("INSERT INTO bulk_user_generations(panel_id,inbound_id,purpose,marker) VALUES($1,1,'POLICY','test-global') RETURNING id::text", panel).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "INSERT INTO bulk_lifecycle_scopes(panel_id,inbound_id,generation_id,enabled,use_global_policy,allow_create,max_batch_size,remaining_operations,expires_at) VALUES($1,1,$2,false,true,true,10,7,now()+interval '1 hour')", panel, generation)
	s := Server{DB: db}
	call := func() int {
		r := httptest.NewRequest("POST", "/", nil).WithContext(context.WithValue(ctx, principalKey{}, auth.Principal{Role: auth.Admin}))
		w := httptest.NewRecorder()
		s.resumeCapacityAutomation(w, r)
		return w.Code
	}
	id, err := (cleanup.Store{DB: db}).Start(ctx, []string{panel})
	if err != nil {
		t.Fatal(err)
	}
	if code := call(); code != 409 {
		t.Fatal("cleanup guard", code)
	}
	if err = (cleanup.Store{DB: db}).Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	if code := call(); code != 409 {
		t.Fatal("disabled policy guard", code)
	}
	sqlMust(t, db, "UPDATE global_config_policies SET enabled=true WHERE policy_key='reality'")
	if code := call(); code != 200 {
		t.Fatal("explicit resume", code)
	}
	var enabled bool
	var budget int
	if err = db.QueryRow("SELECT enabled,remaining_operations FROM bulk_lifecycle_scopes WHERE panel_id=$1", panel).Scan(&enabled, &budget); err != nil || !enabled || budget != 7 {
		t.Fatal(enabled, budget, err)
	}
}
