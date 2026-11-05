package adminapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
)

func TestAccountIssuesBrowser(t *testing.T) {
	if os.Getenv("DOB_RUN_BROWSER_TEST") != "1" {
		t.Skip("browser opt-in")
	}
	db := adminTestDB(t)
	a := ""
	for _, provider := range []string{"digitalocean", "vultr", "linode", "aws", "kamatera", "upcloud", "ovh"} {
		id := seedBuildAccount(t, db, provider)
		if provider == "linode" {
			a = id
		}
		putBuildSnapshot(t, db, id, "active", true, 10, 1)
		sqlMust(t, db, `UPDATE accounts SET name=provider,provider_state='LOCKED',provider_state_detail='Provider lock remains visible',provider_state_at=now(),provider_error_state='PROXY_ERROR',provider_error_detail='{"provider_error":"Independent route error remains visible"}',provider_error_at=now() WHERE id=$1`, id)
		sqlMust(t, db, `INSERT INTO operations(id,account_id,kind,idempotency_key,state,attempt) VALUES(gen_random_uuid(),$1,'CREATE_DROPLET','browser-unknown','unknown',1)`, id)
	}
	// Real accounts have populated server tables and long capacity states. Empty
	// fixtures hid the desktop grid minimum and mobile automatic zoom regressions.
	sqlMust(t, db, `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Inventory,Servers}',
	 (SELECT jsonb_agg(jsonb_build_object('ID','layout-server-'||i,'Name','account-diagnostic-server-'||i,
	 'State','running','Ready',true,'RegionID','us-east','PrimaryIPv4','203.0.113.'||i,
	 'CreatedAt','2026-10-31T12:34:56Z','Tags',jsonb_build_array('diagnostic-fixture')))
	 FROM generate_series(1,10) i)) WHERE account_id=$1`, a)

	s := Server{DB: db}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("../../web/static"))))
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "../../web/static/index.html") })
	mux.HandleFunc("GET /api/v1/accounts", s.accounts)
	mux.HandleFunc("GET /api/v1/accounts/{id}/dashboard", s.accountDashboard)
	mux.HandleFunc("GET /api/v1/proxies", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, []any{}) })
	mux.HandleFunc("POST /api/v1/accounts/{id}/refresh", func(w http.ResponseWriter, r *http.Request) {
		sqlMust(t, db, `UPDATE accounts SET provider_state='TOKEN_INVALID',provider_state_detail='Fresh credential rejection is visible',provider_state_at=now() WHERE id=$1`, r.PathValue("id"))
		writeJSON(w, 502, map[string]any{"error": "provider_discovery_failed", "state": "TOKEN_INVALID", "detail": "Fresh credential rejection is visible"})
	})
	mux.HandleFunc("POST /api/v1/accounts/{id}/preflight", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 409, map[string]any{"ready": false, "checks": map[string]bool{"provider_identity": false, "network": true}, "provider_error": "Preflight provider rejection"})
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{}) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin})))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	root := os.Getenv("DOB_UI_ARTIFACT_DIR")
	if root == "" {
		root = t.TempDir()
	}
	root, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(root, 0700)
	cmd := exec.CommandContext(ctx, "node", "../../web/tests/account-issues-browser.cjs")
	cmd.Env = append(os.Environ(), "DOB_UI_BASE="+server.URL, "DOB_ISSUES_ID="+a, "DOB_UI_ARTIFACT_DIR="+root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser: %v\n%s", err, out)
	}
	t.Log(string(out))
}
