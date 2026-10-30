package adminapi

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func countrySave(s *Server, id, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("PUT", "/", strings.NewReader(body))
	r.SetPathValue("id", id)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin}))
	w := httptest.NewRecorder()
	s.saveProxyCountryPolicy(w, r)
	return w
}

func TestCountryPolicyPersistenceFencingAndLegacyReset(t *testing.T) {
	db := adminTestDB(t)
	a, _ := ruleFixture(t, db, "kamatera", 1)
	s := &Server{DB: db}
	proxy := perfUUID()
	sqlMust(t, db, `INSERT INTO proxies(id,name,type,host,port,status,adapter) VALUES($1,'fixture','http','127.0.0.1',1080,'healthy','suffix-session')`, proxy)
	sqlMust(t, db, `INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,'proxy_required',$2) ON CONFLICT(account_id) DO UPDATE SET mode=EXCLUDED.mode,proxy_id=EXCLUDED.proxy_id`, a, proxy)
	sqlMust(t, db, `INSERT INTO account_proxy_pool(account_id,proxy_id,priority,enabled) VALUES($1,$2,0,true)`, a, proxy)
	sqlMust(t, db, `INSERT INTO account_network_identities(account_id,timezone,locale,preferred_country_code,sticky_session,exit_ip,subnet_key,last_health_ok) VALUES($1,'Europe/Rome','it-IT','it','saved-session','198.51.100.8','198.51.100.0/24',true)`, a)
	r := httptest.NewRequest("GET", "/", nil)
	r.SetPathValue("id", a)
	w := httptest.NewRecorder()
	s.proxyCountryPolicy(w, r)
	var response struct {
		CountryOptions []any `json:"country_options"`
		Policy         struct {
			Revision  int64    `json:"revision"`
			Countries []string `json:"countries"`
		} `json:"policy"`
	}
	if json.Unmarshal(w.Body.Bytes(), &response) != nil || w.Code != 200 || len(response.CountryOptions) != 250 || response.Policy.Revision != 0 || response.Policy.Countries[0] != "AUTO" {
		t.Fatal(w.Code, w.Body.String())
	}
	before, err := s.accountSettingsSnapshot(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	w = countrySave(s, a, `{"revision":0,"countries":["US"]}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	after, err := s.accountSettingsSnapshot(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(before)
	v, _ := json.Marshal(after)
	if string(b) != string(v) {
		t.Fatal("country change rewrote build or routing settings")
	}
	var learned, session, ip string
	var health bool
	if err := db.QueryRow(`SELECT preferred_country_code,sticky_session,host(exit_ip),last_health_ok FROM account_network_identities WHERE account_id=$1`, a).Scan(&learned, &session, &ip, &health); err != nil || learned != "it" || session != "saved-session" || ip != "198.51.100.8" || health {
		t.Fatal("lost legacy intent or falsely healthy", learned, session, ip, health, err)
	}
	if _, err := db.Exec(`UPDATE account_network_identities SET last_health_ok=true WHERE account_id=$1`, a); err == nil {
		t.Fatal("old keeper bypassed country policy")
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = app.FenceCountryWriterTx(context.Background(), tx, a); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE account_network_identities SET last_health_ok=true,selected_target_country='US',country_code='us' WHERE account_id=$1`, a); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE account_proxy_selection SET applied_revision=1,active_stage='US',candidate='{"fixture":true}' WHERE account_id=$1`, a); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, `UPDATE proxies SET last_checked_at=now() WHERE id=$1`, proxy)
	var applied int64
	if err := db.QueryRow(`SELECT applied_revision FROM account_proxy_selection WHERE account_id=$1`, a).Scan(&applied); err != nil || applied != 1 {
		t.Fatal("health timestamp invalidated intent", applied, err)
	}
	sqlMust(t, db, `UPDATE proxies SET username='new-credential-scope' WHERE id=$1`, proxy)
	var discarded bool
	if err := db.QueryRow(`SELECT applied_revision=0 AND candidate IS NULL AND legacy_lease_until IS NULL FROM account_proxy_selection WHERE account_id=$1`, a).Scan(&discarded); err != nil || !discarded {
		t.Fatal("stale credential proof survived", discarded, err)
	}
	w = countrySave(s, a, `{"revision":0,"countries":["TR"]}`)
	if w.Code != 409 {
		t.Fatal("stale policy accepted", w.Code)
	}
	// A schema rollback must not erase a saved explicit policy.
	down, err := os.ReadFile("../../migrations/000204_account_proxy_country_policy.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	rollback, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	_, err = rollback.Exec(string(down))
	rollback.Rollback()
	if err == nil {
		t.Fatal("downgrade erased explicit intent")
	}
	w = countrySave(s, a, `{"revision":1,"countries":["AUTO"]}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var target *string
	if err := db.QueryRow(`SELECT preferred_country_code,selected_target_country,last_health_ok FROM account_network_identities WHERE account_id=$1`, a).Scan(&learned, &target, &health); err != nil || learned != "it" || target != nil || health {
		t.Fatal("AUTO reset lost learned preference", learned, target, health, err)
	}
	if w := countrySave(s, a, `{"revision":2,"countries":["US","DE","FR"]}`); w.Code != 200 {
		t.Fatal("three countries rejected", w.Code, w.Body.String())
	}
	var saved []byte
	if e := db.QueryRow(`SELECT countries FROM account_proxy_country_policies WHERE account_id=$1`, a).Scan(&saved); e != nil {
		t.Fatal(e)
	}
	var ordered []string
	if json.Unmarshal(saved, &ordered) != nil || strings.Join(ordered, ",") != "US,DE,FR" {
		t.Fatal("priority order lost", string(saved))
	}
	for _, body := range []string{`{"revision":2}`, `{"revision":2,"countries":["US","TR","DE","FR"]}`, `{"revision":2,"countries":["US","US"]}`, `{"revision":2,"countries":["ZZ"]}`, `{"revision":2,"countries":["TR"],"unknown_field":true}`} {
		if w := countrySave(s, a, body); w.Code != 400 {
			t.Fatal("invalid intent accepted", body, w.Code)
		}
	}
}
