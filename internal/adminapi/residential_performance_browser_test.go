package adminapi

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPerformanceBrowser(t *testing.T) {
	if os.Getenv("DOB_RUN_BROWSER_TEST") != "1" {
		t.Skip("browser acceptance opt-in")
	}
	db := adminTestDB(t)
	_, _, panel := seedPanel(t, db, "http://canary.test")
	sqlMust(t, db, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,status,last_success_at,outbound_tag) VALUES($1,'ads-fixture','socks5','localhost',1080,'healthy',now(),'residential-ads-fixture')", perfUUID())
	sqlMust(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	sqlMust(t, db, "INSERT INTO panel_routing_state(panel_id,revision,plan_hash,state,pool_enabled,verified_at) SELECT $1,revision,'fixture','APPLIED',true,now() FROM residential_routing_control", panel)
	secret, _ := secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
	s := Server{DB: db, Container: app.Container{Secrets: secret}}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("../../web/static"))))
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "../../web/static/index.html") })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]bool{"ready": true}) })
	mux.HandleFunc("GET /api/v1/residential-performance", s.residentialPerformanceStatus)
	mux.HandleFunc("POST /api/v1/residential-performance", s.residentialPerformanceAction)
	mux.HandleFunc("GET /api/v1/residential-routing", s.residentialRoutingStatus)
	mux.HandleFunc("GET /api/v1/residential-proxies", s.residentialProxies)
	mux.HandleFunc("POST /api/v1/residential-proxies/import", s.importResidentialProxies)
	mux.HandleFunc("POST /api/v1/residential-proxies/delete", s.deleteResidentialProxies)
	mux.HandleFunc("POST /api/v1/residential-proxies/export", s.exportResidentialProxies)
	// Isolated fixture identity, never a production token or service.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin}))
		mux.ServeHTTP(w, r)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "../../web/tests/residential_performance.cjs")
	root := os.Getenv("DOB_UI_ARTIFACT_DIR")
	if root == "" {
		root = t.TempDir()
	}
	root, _ = filepath.Abs(root)
	cmd.Env = append(os.Environ(), "DOB_UI_BASE="+server.URL, "DOB_UI_ARTIFACT_DIR="+root)
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("browser: %v %s", e, out)
	}
	t.Log(string(out))
	var count int
	db.QueryRow("SELECT count(*) FROM residential_performance_experiments").Scan(&count)
	if count != 1 {
		t.Fatal("duplicate experiment after lost response", count)
	}
	var state string
	db.QueryRow("SELECT state FROM residential_performance_experiments").Scan(&state)
	if state != "ROLLING_BACK" {
		t.Fatal("rollback not requested", state)
	}
	var desired []byte
	db.QueryRow("SELECT config FROM residential_performance_panels WHERE panel_id=$1", panel).Scan(&desired)
	if len(desired) != 0 {
		t.Fatal("prior settings not requested")
	}
}
