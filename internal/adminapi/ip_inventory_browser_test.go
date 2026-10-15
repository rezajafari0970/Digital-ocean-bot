package adminapi

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/ipam"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestIPInventoryBrowser(t *testing.T) {
	if os.Getenv("DOB_RUN_BROWSER_TEST") != "1" {
		t.Skip("browser opt-in")
	}
	db := adminTestDB(t)
	ipSeed(t, db)
	st := ipam.InventoryStore{DB: db}
	ticket, e := st.Begin(context.Background(), ipa)
	if e != nil {
		t.Fatal(e)
	}
	n := 0
	o := ipObs(&n)
	if e = st.Finish(context.Background(), ticket, &o, "", 0, 0); e != nil {
		t.Fatal(e)
	}
	s := Server{DB: db}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/ip-inventory", s.ipInventory)
	mux.HandleFunc("PUT /api/v1/accounts/{id}/ip-policy/{family}", s.saveIPPolicy)
	mux.HandleFunc("PUT /api/v1/accounts/{id}/ip-funding", s.saveIPFunding)
	mux.HandleFunc("GET /api/v1/auth/me", s.authMe)
	mux.HandleFunc("POST /api/v1/accounts/{id}/ip-inventory/refresh", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"state": "FAILED", "error_class": "rate_limited"})
	})
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("../../web/static"))))
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "../../web/static/index.html") })
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{}) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin})))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "../../web/tests/ip-inventory.cjs")
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
}
