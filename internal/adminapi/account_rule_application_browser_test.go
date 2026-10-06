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

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
)

func TestAccountRuleApplicationBrowser(t *testing.T) {
	if os.Getenv("DOB_RUN_BROWSER_TEST") != "1" {
		t.Skip("browser opt-in")
	}
	db := adminTestDB(t)
	a, _ := ruleFixture(t, db, "digitalocean", 3)
	s := Server{DB: db}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("../../web/static"))))
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "../../web/static/index.html") })
	mux.HandleFunc("GET /api/v1/accounts", s.accounts)
	mux.HandleFunc("PUT /api/v1/accounts/{id}", s.updateAccount)
	mux.HandleFunc("GET /api/v1/proxies", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, []any{}) })
	mux.HandleFunc("GET /api/v1/accounts/{id}/options", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"regions": []any{map[string]any{"ID": "ams", "Name": "Amsterdam", "Available": true}},
			"plans":  []any{map[string]any{"ID": "small", "Available": true, "MemoryMB": 1024, "CPU": 1, "DiskGB": 25}},
			"images": []any{map[string]any{"ID": "ubuntu-24-04-x64", "Family": "ubuntu", "Version": "24.04", "Available": true}}})
	})
	mux.HandleFunc("POST /__fixture/tick", func(w http.ResponseWriter, r *http.Request) {
		if err := (app.Container{DB: db}).ProcessAccountRuleApplication(r.Context(), a); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		var retiring int
		db.QueryRow("SELECT count(*) FROM droplets WHERE account_id=$1 AND state='RETIRING'", a).Scan(&retiring)
		writeJSON(w, 200, map[string]any{"retiring": retiring})
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
	cmd := exec.CommandContext(ctx, "node", "../../web/tests/account-rule-application.cjs")
	cmd.Env = append(os.Environ(), "DOB_UI_BASE="+server.URL, "DOB_RULE_ID="+a, "DOB_UI_ARTIFACT_DIR="+root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser: %v\n%s", err, out)
	}
	t.Log(string(out))
}
