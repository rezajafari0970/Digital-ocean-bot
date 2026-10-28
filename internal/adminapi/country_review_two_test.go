package adminapi

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCountryReviewTwoAccountListOneConnectionPostgres(t *testing.T) {
	db := adminTestDB(t)
	ruleFixture(t, db, "kamatera", 1)
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	s := &Server{DB: db}
	s.accounts(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"settings_revision"`) {
		t.Fatal("nonempty listing exhausted SQL pool", w.Code, w.Body.String())
	}
}
func TestCountryReviewTwoUUIDAliasUsesSameRouteLockPostgres(t *testing.T) {
	db := adminTestDB(t)
	a, _ := ruleFixture(t, db, "kamatera", 1)
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock_shared(hashtextextended($1,0))`, "account-route:"+a); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(ctx, `SELECT pg_advisory_unlock_shared(hashtextextended($1,0))`, "account-route:"+a)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- countrySave(&Server{DB: db}, strings.ToUpper(a), `{"revision":0,"countries":["US"]}`)
	}()
	select {
	case w := <-done:
		t.Fatal("UUID alias bypassed route lock", w.Code, w.Body.String())
	case <-time.After(120 * time.Millisecond):
	}
	if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_unlock_shared(hashtextextended($1,0))`, "account-route:"+a); err != nil {
		t.Fatal(err)
	}
	select {
	case w := <-done:
		if w.Code != 409 || !strings.Contains(w.Body.String(), "proxy_country_requires_proxy") {
			t.Fatal(w.Code, w.Body.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canonical route writer blocked")
	}
}
func TestCountryReviewTwoInvalidModeCannotValidateReplacementPostgres(t *testing.T) {
	db := adminTestDB(t)
	a, _ := ruleFixture(t, db, "kamatera", 1)
	s := &Server{DB: db}
	// No provider registry is available: reaching token validation would return a
	// different error (and before the fix could open a direct transport).
	w := settingsRequest(s, a, `{"settings_revision":1,"network_mode":"proxy_requird","token":"fixture-never-dispatch"}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "invalid_network_mode") {
		t.Fatal("invalid mode passed token boundary", w.Code, w.Body.String())
	}
	if _, err := s.validateReplacementToken(context.Background(), a, "kamatera", "fixture", "invalid", ""); err == nil || err.Error() != "invalid_network_mode" {
		t.Fatal("validator accepted invalid mode", err)
	}
}

func TestCountryReviewTwoAccountProxyObservationCASPostgres(t *testing.T) {
	for _, mutation := range []string{"endpoint", "secret"} {
		t.Run(mutation, func(t *testing.T) {
			db := adminTestDB(t)
			ctx := context.Background()
			a, _ := ruleFixture(t, db, "kamatera", 1)
			p := perfUUID()
			sqlMust(t, db, `INSERT INTO proxies(id,name,type,host,port,status,secret_ref) VALUES($1,'probe fixture','http','127.0.0.1',1080,'healthy','fixture-secret')`, p)
			sqlMust(t, db, `INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,'proxy_required',$2) ON CONFLICT(account_id) DO UPDATE SET mode=EXCLUDED.mode,proxy_id=EXCLUDED.proxy_id`, a, p)
			store, err := secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
			if err != nil {
				t.Fatal(err)
			}
			if err = store.PutProxy(ctx, p, "fixture-secret", "proxy_password", []byte("old")); err != nil {
				t.Fatal(err)
			}
			cfg, err := (app.Repository{DB: db}).Account(ctx, a)
			if err != nil {
				t.Fatal(err)
			}
			if mutation == "endpoint" {
				sqlMust(t, db, `UPDATE proxies SET host='127.0.0.2',settings_revision=settings_revision+1 WHERE id=$1`, p)
			} else {
				if err = store.PutProxy(ctx, p, "fixture-secret", "proxy_password", []byte("new")); err != nil {
					t.Fatal(err)
				}
			}
			s := &Server{DB: db}
			for _, observationErr := range []error{nil, errors.New("old auth failure")} {
				if err = s.saveAccountProxyObservation(ctx, cfg, ProxyObservation{IP: "198.18.97.5", CountryCode: "US", Country: "US", Timezone: "UTC"}, observationErr); !errors.Is(err, errProxyObservationStale) {
					t.Fatal("stale probe published", err)
				}
			}
			var pristine bool
			if err = db.QueryRow(`SELECT last_checked_at IS NULL AND failure_count=0 AND exit_ip IS NULL FROM proxies WHERE id=$1`, p).Scan(&pristine); err != nil || !pristine {
				t.Fatal("old result changed replacement health", pristine, err)
			}
		})
	}
}
