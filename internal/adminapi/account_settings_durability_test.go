package adminapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
)

func settingsRequest(s *Server, id, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("PUT", "/", strings.NewReader(body))
	r.SetPathValue("id", id)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin}))
	w := httptest.NewRecorder()
	s.updateAccount(w, r)
	return w
}

func TestAccountSettingsOmissionsConflictAndNoSilentClamp(t *testing.T) {
	db := adminTestDB(t)
	a, _ := ruleFixture(t, db, "kamatera", 3)
	s := &Server{DB: db}
	sqlMust(t, db, `UPDATE accounts SET preferred_regions='["US-CH","US-AT","US-SC","US-NY2","US-MI2"]', preferred_sizes='["first","second"]', preferred_images='["ubuntu26","ubuntu24"]',preferred_image='ubuntu26',fallback_any_region=false,auto_batch_size=2,auto_interval_seconds=777,next_build_at=now()+interval '2 hours' WHERE id=$1`, a)
	before, err := s.accountSettingsSnapshot(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	w := settingsRequest(s, a, `{"name":"renamed","settings_revision":1}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	after, err := s.accountSettingsSnapshot(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	var scheduled bool
	if err := db.QueryRow(`SELECT next_build_at>now()+interval '1 hour' FROM accounts WHERE id=$1`, a).Scan(&scheduled); err != nil || !scheduled {
		t.Fatal("rename reset schedule", err)
	}
	before.Name = "renamed"
	before.SettingsRevision = 2
	b, _ := json.Marshal(before)
	v, _ := json.Marshal(after)
	if string(b) != string(v) {
		t.Fatalf("omitted settings changed\nbefore %s\nafter %s", b, v)
	}
	w = settingsRequest(s, a, `{"name":"stale unversioned client"}`)
	if w.Code != 409 {
		t.Fatal("unversioned draft accepted", w.Code, w.Body.String())
	}
	w = settingsRequest(s, a, `{"name":"stale","settings_revision":1}`)
	if w.Code != 409 {
		t.Fatal("stale edit accepted", w.Code, w.Body.String())
	}
	w = settingsRequest(s, a, `{"settings_revision":2,"name":"invalid lifetime","lifetime_min_seconds":60,"lifetime_max_seconds":120}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "30_minutes") {
		t.Fatal("silent clamp", w.Code, w.Body.String())
	}
	sqlMust(t, db, `UPDATE schedules SET enabled=false WHERE account_id=$1`, a)
	sqlMust(t, db, `UPDATE deployment_profiles SET enabled=false WHERE account_id=$1`, a)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- settingsRequest(s, a, `{"name":"concurrent","settings_revision":2}`).Code
		}()
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatal("concurrent lost update", counts)
	}
	var enabled bool
	if err := db.QueryRow(`SELECT bool_or(s.enabled OR p.enabled) FROM schedules s JOIN deployment_profiles p ON p.id=s.profile_id WHERE s.account_id=$1`, a).Scan(&enabled); err != nil || enabled {
		t.Fatal("rename re-enabled automation", enabled, err)
	}
	// JSON decoding reuses slice backing storage. A separate before-snapshot is
	// essential, otherwise equal-length edits look unchanged to the scheduler.
	sqlMust(t, db, `UPDATE accounts SET next_build_at=now()+interval '2 hours' WHERE id=$1`, a)
	w = settingsRequest(s, a, `{"settings_revision":3,"regions":["US-AT","US-CH","US-SC","US-NY2","US-MI2"],"sizes":["replacement","second"]}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var reset bool
	var region, size string
	if err := db.QueryRow(`SELECT a.next_build_at IS NULL,p.config->>'region',p.config->>'size' FROM accounts a JOIN deployment_profiles p ON p.account_id=a.id AND p.name='account-auto' WHERE a.id=$1`, a).Scan(&reset, &region, &size); err != nil || !reset || region != "US-AT" || size != "replacement" {
		t.Fatal("saved equal-length arrays not applied to schedule", reset, region, size, err)
	}

}

func TestAccountSettingsPreserveActiveFailover(t *testing.T) {
	db := adminTestDB(t)
	a, _ := ruleFixture(t, db, "kamatera", 3)
	p1, p2 := perfUUID(), perfUUID()
	sqlMust(t, db, `INSERT INTO proxies(id,name,type,host,port,status) VALUES($1,'primary','http','127.0.0.1',1080,'down'),($2,'fallback','http','127.0.0.1',1081,'healthy')`, p1, p2)
	sqlMust(t, db, `INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,'proxy_required',$2) ON CONFLICT(account_id) DO UPDATE SET mode=EXCLUDED.mode,proxy_id=EXCLUDED.proxy_id`, a, p2)
	sqlMust(t, db, `INSERT INTO account_proxy_pool(account_id,proxy_id,priority,enabled) VALUES($1,$2,0,true),($1,$3,1,true)`, a, p1, p2)
	sqlMust(t, db, `INSERT INTO account_network_identities(account_id,timezone,locale,exit_ip,subnet_key,last_health_ok) VALUES($1,'UTC','en-US','198.51.100.8','198.51.100.0/24',true)`, a)
	s := &Server{DB: db}
	w := settingsRequest(s, a, `{"name":"keep my route","settings_revision":1}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var active, ip string
	if err := db.QueryRow(`SELECT np.proxy_id::text,host(ani.exit_ip) FROM network_profiles np JOIN account_network_identities ani USING(account_id) WHERE account_id=$1`, a).Scan(&active, &ip); err != nil || active != p2 || ip != "198.51.100.8" {
		t.Fatal("unrelated edit reset failover", active, ip, err)
	}
}

func TestProvisionDiagnosticsBeforeSSH(t *testing.T) {
	db := adminTestDB(t)
	a, _ := ruleFixture(t, db, "kamatera", 1)
	id := perfUUID()
	profile := perfUUID()
	sqlMust(t, db, `INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$2,'diagnostics','{}')`, profile, a)
	sqlMust(t, db, `INSERT INTO deployments(id,account_id,profile_id,state,current_step,attempt,last_error) VALUES($1,$2,$3,'CREATING','create',176,'provider confirmation pending')`, id, a, profile)
	s := &Server{DB: db}
	for _, found := range []bool{true, false} {
		r := httptest.NewRequest("GET", "/", nil)
		v := id
		if !found {
			v = perfUUID()
		}
		r.SetPathValue("id", v)
		w := httptest.NewRecorder()
		s.provisionActivity(w, r)
		if !found {
			if w.Code != 404 {
				t.Fatal(w.Code, w.Body.String())
			}
			continue
		}
		var body map[string]any
		if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != 200 || body["provisioning_started"] != false || body["attempt"] != float64(176) || body["state"] != "CREATING" {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

func TestOrdinaryAccountCreationIsAtomic(t *testing.T) {
	s, _ := authorizationTestServer(t)
	sqlMust(t, s.DB, `INSERT INTO installers(name,version,manifest,sha256,active) VALUES('atomic-create',1,'{"capabilities":["xui_database","xui_panel"]}',repeat('a',64),true)`)
	body := accountWrite{Provider: "ovh", Token: "isolated-fixture-credential", Name: "atomic ordinary create", ExternalID: "isolated-project", Regions: []string{"GRA11"}, Sizes: []string{"small"}, Images: []string{"ubuntu"}, NetworkMode: "direct", LifetimeMinSeconds: 1800, LifetimeMaxSeconds: 3600, BuildSpacingMinutes: 1, BuildSpacingMaxMinutes: 2, DesiredServerCount: 1}
	sqlMust(t, s.DB, `CREATE FUNCTION reject_account_profile() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'isolated late-write fault'; END $$; CREATE TRIGGER reject_account_profile BEFORE INSERT ON network_profiles FOR EACH ROW EXECUTE FUNCTION reject_account_profile()`)
	w := authorizationRequest(s, "owner", "POST", "/save", body)
	if w.Code != 500 {
		t.Fatal(w.Code, w.Body.String())
	}
	var count int
	if err := s.DB.QueryRow(`SELECT (SELECT count(*) FROM accounts WHERE external_id='isolated-project')+(SELECT count(*) FROM secrets WHERE kind='ovh_credential')`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial account/secret survived", count, err)
	}
	sqlMust(t, s.DB, `DROP TRIGGER reject_account_profile ON network_profiles`)
	w = authorizationRequest(s, "owner", "POST", "/save", body)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := s.DB.QueryRow(`SELECT count(*) FROM accounts a JOIN secrets s ON s.account_id=a.id JOIN network_profiles n ON n.account_id=a.id WHERE a.external_id='isolated-project'`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestProxyNameSavePreservesIdentityAndConflicts(t *testing.T) {
	db := adminTestDB(t)
	a, _ := ruleFixture(t, db, "kamatera", 1)
	id := perfUUID()
	sqlMust(t, db, `INSERT INTO proxies(id,name,type,host,port,status,adapter,username) VALUES($1,'old proxy','http','127.0.0.1',1080,'down','suffix-session','private-base')`, id)
	sqlMust(t, db, `INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,'proxy_required',$2) ON CONFLICT(account_id) DO UPDATE SET mode=EXCLUDED.mode,proxy_id=EXCLUDED.proxy_id`, a, id)
	sqlMust(t, db, `INSERT INTO account_proxy_pool(account_id,proxy_id,priority) VALUES($1,$2,0)`, a, id)
	sqlMust(t, db, `INSERT INTO account_network_identities(account_id,sticky_session,exit_ip,subnet_key,last_health_ok) VALUES($1,'must-survive','198.18.30.8','198.18.30.0/24',true)`, a)
	s := &Server{DB: db}
	save := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PUT", "/", strings.NewReader(body))
		r.SetPathValue("id", id)
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin}))
		w := httptest.NewRecorder()
		s.updateProxy(w, r)
		return w
	}
	w := save(`{"name":"renamed proxy","settings_revision":1}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var session, ip, status, user string
	var revision int64
	if err := db.QueryRow(`SELECT i.sticky_session,host(i.exit_ip),p.status,p.username,p.settings_revision FROM account_network_identities i JOIN network_profiles n USING(account_id) JOIN proxies p ON p.id=n.proxy_id WHERE i.account_id=$1`, a).Scan(&session, &ip, &status, &user, &revision); err != nil {
		t.Fatal(err)
	}
	if session != "must-survive" || ip != "198.18.30.8" || status != "down" || user != "private-base" || revision != 2 {
		t.Fatal("rename changed route or credentials", session, ip, status, user, revision)
	}
	w = save(`{"name":"unversioned proxy draft"}`)
	if w.Code != 409 {
		t.Fatal("unversioned proxy draft accepted", w.Code, w.Body.String())
	}
	w = save(`{"name":"stale draft","settings_revision":1}`)
	if w.Code != 409 {
		t.Fatal("concurrent proxy draft overwrote settings", w.Code, w.Body.String())
	}
}
