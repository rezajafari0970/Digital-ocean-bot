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

func TestCleanupBulkJavaScript(t *testing.T) {
	cmd := exec.Command("node", "../../web/tests/cleanup-bulk.cjs")
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, out)
	}
}
func TestBulkCleanupBrowser(t *testing.T) {
	if os.Getenv("DOB_RUN_BROWSER_TEST") != "1" {
		t.Skip("browser opt-in")
	}
	db := adminTestDB(t)
	other, _, _ := seedPanel(t, db, "http://active.invalid")
	for _, state := range []string{"ACTIVE", "TOKEN_INVALID"} {
		a, _, _ := seedPanel(t, db, "http://cleanup.invalid")
		sqlMust(t, db, "UPDATE accounts SET enabled=false,runtime_status='DELETE_PENDING',deletion_requested_at=now()-interval '3 minutes',provider_state=$2,provider_state_detail='cleanup authorization detail' WHERE id=$1", a, state)
		sqlMust(t, db, "INSERT INTO account_deletion_jobs(account_id) VALUES($1)", a)
		sqlMust(t, db, "INSERT INTO network_profiles(id,account_id,mode) VALUES(gen_random_uuid(),$1,'direct')", a)
	}
	sqlMust(t, db, "INSERT INTO network_profiles(id,account_id,mode) VALUES(gen_random_uuid(),$1,'direct')", other)
	s := Server{DB: db}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("../../web/static"))))
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "../../web/static/index.html") })
	mux.HandleFunc("GET /api/v1/accounts", s.accounts)
	mux.HandleFunc("POST /api/v1/accounts/{id}/purge", s.purgeAccount)
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
	root, _ = filepath.Abs(root)
	os.MkdirAll(root, 0700)
	cmd := exec.CommandContext(ctx, "node", "../../web/tests/account-deletion.cjs")
	cmd.Env = append(os.Environ(), "DOB_UI_BASE="+server.URL, "DOB_UI_ARTIFACT_DIR="+root, "DOB_BULK_CLEANUP=1", "DOB_OTHER_ID="+other)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("bulk browser %v %s", e, out)
	} else {
		t.Log(string(out))
	}
	var n int
	if e := db.QueryRow("SELECT count(*) FROM accounts").Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if e := db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", other).Scan(&n); e != nil || n != 1 {
		t.Fatal("active account changed", n, e)
	}
}
