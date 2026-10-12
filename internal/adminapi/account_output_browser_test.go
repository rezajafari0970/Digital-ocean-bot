package adminapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAccountOutputBrowser(t *testing.T) {
	if os.Getenv("DOB_RUN_BROWSER_TEST") != "1" {
		t.Skip("browser opt-in")
	}
	db := adminTestDB(t)
	fixtures := seedAccountOutput(t, db)
	s := &Server{DB: db}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("../../web/static"))))
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "../../web/static/index.html") })
	mux.HandleFunc("GET /api/v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		var rows []map[string]any
		for _, f := range fixtures {
			rows = append(rows, map[string]any{"id": f.Account, "name": "Same account name", "provider_state": "ACTIVE", "enabled": true})
		}
		writeJSON(w, 200, rows)
	})
	mux.HandleFunc("POST /api/v1/accounts/{id}/output/share", s.createAccountOutputShare)
	mux.HandleFunc("GET /share/account-output/{token}", s.sharedAccountOutput)
	mux.HandleFunc("POST /__fixture/expire", func(w http.ResponseWriter, r *http.Request) {
		sqlMust(t, db, "UPDATE output_config_snapshots SET visible_until=now()-interval '1 second'")
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{}) })
	server := httptest.NewServer(mux)
	defer server.Close()
	root := os.Getenv("DOB_UI_ARTIFACT_DIR")
	if root == "" {
		root = t.TempDir()
	}
	root, _ = filepath.Abs(root)
	os.MkdirAll(root, 0700)
	raw, _ := json.Marshal(fixtures)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "../../web/tests/account-output.cjs")
	cmd.Env = append(os.Environ(), "DOB_UI_BASE="+server.URL, "DOB_UI_ARTIFACT_DIR="+root, "DOB_ACCOUNT_FIXTURES="+string(raw))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser %v %s", err, out)
	}
	t.Log(string(out))
	var n int
	if err = db.QueryRow("SELECT count(*) FROM account_output_share_tokens").Scan(&n); err != nil || n != 8 {
		t.Fatal("refresh minted tokens", n, err)
	}
}
