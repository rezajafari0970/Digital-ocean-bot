package adminapi

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestServerProtectionBrowser(t *testing.T) {
	if os.Getenv("DOB_RUN_BROWSER_TEST") != "1" {
		t.Skip("browser opt-in")
	}
	db := adminTestDB(t)
	seedPanel(t, db, "http://fixture.test:2053")
	s := Server{DB: db}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("../../web/static"))))
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "../../web/static/index.html") })
	mux.HandleFunc("GET /api/v1/server-protection", s.serverProtectionStatus)
	mux.HandleFunc("POST /api/v1/server-protection", s.serverProtectionAction)
	mux.HandleFunc("GET /api/v1/configs", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, []any{}) })
	mux.HandleFunc("GET /api/v1/config-capacity/cleanup/current", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, nil) })
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{}) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin})))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "../../web/tests/server-protection.cjs")
	root := os.Getenv("DOB_UI_ARTIFACT_DIR")
	if root == "" {
		root = t.TempDir()
	}
	root, _ = filepath.Abs(root)
	cmd.Env = append(os.Environ(), "DOB_UI_BASE="+server.URL, "DOB_UI_ARTIFACT_DIR="+root)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("browser %v %s", e, out)
	} else {
		t.Log(string(out))
	}
	var enabled bool
	var revision int
	if e := db.QueryRow("SELECT enabled,revision FROM server_protection_control").Scan(&enabled, &revision); e != nil || enabled || revision != 4 {
		t.Fatal(enabled, revision, e)
	}
	var count int
	db.QueryRow("SELECT count(*) FROM server_protection_requests").Scan(&count)
	if count != 3 {
		t.Fatal("duplicate after lost response", count)
	}
}
