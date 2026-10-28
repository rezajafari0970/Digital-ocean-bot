package adminapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
)

func TestStrictCountryOnboardingAtomicAndBindingPostgres(t *testing.T) {
	s, factory := authorizationTestServer(t)
	sqlMust(t, s.DB, `INSERT INTO installers(name,version,manifest,sha256,active) VALUES('strict-create',1,'{"capabilities":["xui_database","xui_panel"]}',repeat('a',64),true)`)
	body := accountWrite{Provider: "ovh", Token: "fixture-credential", Name: "strict country", ExternalID: "strict-project", Regions: []string{"GRA11"}, Sizes: []string{"small"}, Images: []string{"ubuntu"}, NetworkMode: "direct", ProxyCountry: "US", LifetimeMinSeconds: 1800, LifetimeMaxSeconds: 3600, BuildSpacingMinutes: 1, BuildSpacingMaxMinutes: 2, DesiredServerCount: 1}
	if w := authorizationRequest(s, "owner", "POST", "/save", body); w.Code != 400 {
		t.Fatal("explicit direct creation accepted", w.Code)
	}
	p := perfUUID()
	sqlMust(t, s.DB, `INSERT INTO proxies(id,name,type,host,port,status,adapter) VALUES($1,'country fixture','http','127.0.0.1',1080,'healthy','generic')`, p)
	// A generic proxy cannot claim a country before authorization dispatch.
	w := authorizationRequest(s, "owner", "POST", "/start", map[string]string{"id": authorizationFixtureID, "token": "fixture", "proxy_id": p, "proxy_country": "US"})
	if w.Code != 409 || factory.calls.Load() != 0 {
		t.Fatal("authorization crossed unverified country", w.Code, factory.calls.Load())
	}
	var saved string
	if err := s.DB.QueryRow(`SELECT proxy_country FROM provider_authorizations WHERE id=$1`, authorizationFixtureID).Scan(&saved); err != nil || saved != "US" {
		t.Fatal("authorization intent not country bound", saved, err)
	}
	body.NetworkMode = "proxy_required"
	body.ProxyID = p
	body.ProxyIDs = []string{p}
	// Failure of the country write rolls back account, secret and automation too.
	sqlMust(t, s.DB, `CREATE FUNCTION reject_country_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'country fixture'; END $$; CREATE TRIGGER reject_country_fixture BEFORE INSERT ON account_proxy_country_policies FOR EACH ROW EXECUTE FUNCTION reject_country_fixture()`)
	if w = authorizationRequest(s, "owner", "POST", "/save", body); w.Code != 500 {
		t.Fatal(w.Code, w.Body.String())
	}
	var count int
	if err := s.DB.QueryRow(`SELECT count(*) FROM accounts WHERE external_id='strict-project'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial country account survived", count, err)
	}
	sqlMust(t, s.DB, `DROP TRIGGER reject_country_fixture ON account_proxy_country_policies`)
	w = authorizationRequest(s, "owner", "POST", "/save", body)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var id string
	if err := s.DB.QueryRow(`SELECT a.id::text,p.countries->>0 FROM accounts a JOIN account_proxy_country_policies p ON p.account_id=a.id WHERE a.external_id='strict-project'`).Scan(&id, &saved); err != nil || saved != "US" {
		t.Fatal("created account lost country", saved, err)
	}
	if w = settingsRequest(s, id, `{"settings_revision":1,"network_mode":"direct","proxy_ids":[],"proxy_id":""}`); w.Code != 409 || !strings.Contains(w.Body.String(), "proxy_country_requires_proxy") {
		t.Fatal("settings bypassed country", w.Code, w.Body.String())
	}
	if w = settingsRequest(s, id, `{"settings_revision":1,"network_mode":"direct","proxy_ids":[],"proxy_id":"","token":"never-dispatch"}`); w.Code != 409 || !strings.Contains(w.Body.String(), "credential_route_change_requires_save") {
		t.Fatal("credential tested on uncommitted route", w.Code, w.Body.String())
	}
	// The preview boundary also blocks before any driver discovery/account call.
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"provider":"ovh","token":"fixture","proxy_country":"US"}`))
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin}))
	recorder := httptest.NewRecorder()
	s.runAccountPreview(recorder, r)
	if recorder.Code != 409 || !strings.Contains(recorder.Body.String(), "proxy_country_requires_proxy") {
		t.Fatal("preview silently used direct", recorder.Code)
	}
}
