package adminapi

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAccountDeletionBrowser(t *testing.T) {
	if os.Getenv("DOB_RUN_BROWSER_TEST") != "1" {
		t.Skip("browser opt-in")
	}
	db := adminTestDB(t)
	a, d, _ := seedPanel(t, db, "http://fixture.invalid")
	sqlMust(t, db, "UPDATE accounts SET name='Delete fixture',provider='upcloud' WHERE id=$1", a)
	sqlMust(t, db, "UPDATE droplets SET state='DELETED' WHERE id=$1", d)
	b := perfUUID()
	sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'Other fixture','vultr','fixture')", b)
	for _, id := range []string{a, b} {
		sqlMust(t, db, "INSERT INTO network_profiles(id,account_id,mode) VALUES(gen_random_uuid(),$1,'direct')", id)
	}
	reg := providers.NewRegistry()
	if e := reg.Register(deletionFixtureFactory{&deletionFixtureDriver{provider: "upcloud"}}); e != nil {
		t.Fatal(e)
	}
	c := app.Container{DB: db, Accounts: app.Repository{DB: db}, Providers: reg}
	s := Server{DB: db}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("../../web/static"))))
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "../../web/static/index.html") })
	mux.HandleFunc("GET /api/v1/accounts", s.accounts)
	mux.HandleFunc("DELETE /api/v1/accounts/{id}", s.deleteAccount)
	// Only this ephemeral fixture can advance the clock; no live credentials.
	mux.HandleFunc("POST /__fixture/finish", func(w http.ResponseWriter, r *http.Request) {
		if _, e := db.Exec("UPDATE accounts SET deletion_requested_at=now()-interval '3 minutes' WHERE deletion_requested_at IS NOT NULL"); e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		if _, e := db.Exec("UPDATE account_deletion_jobs SET requested_at=now()-interval '3 minutes',next_attempt_at=now()"); e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		if e := c.ProcessAccountDeletions(r.Context()); e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{}) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin})))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "../../web/tests/account-deletion.cjs")
	root := os.Getenv("DOB_UI_ARTIFACT_DIR")
	if root == "" {
		root = t.TempDir()
	}
	root, _ = filepath.Abs(root)
	cmd.Env = append(os.Environ(), "DOB_UI_BASE="+server.URL, "DOB_UI_ARTIFACT_DIR="+root, "DOB_DELETE_ID="+a, "DOB_OTHER_ID="+b)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("browser %v %s", e, out)
	} else {
		t.Log(string(out))
	}
	var n int
	if e := db.QueryRow("SELECT count(*) FROM accounts").Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
}
