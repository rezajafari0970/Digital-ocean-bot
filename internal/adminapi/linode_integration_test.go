package adminapi

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/linode"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/upcloud"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/vultr"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLinodeInventoryOwnershipAndIncompleteSnapshot(t *testing.T) {
	db := adminTestDB(t)
	account, droplet, _ := seedPanel(t, db, "http://linode.invalid")
	other, _, _ := seedPanel(t, db, "http://other.invalid")
	sqlMust(t, db, "UPDATE accounts SET provider='linode' WHERE id IN($1,$2)", account, other)
	sqlMust(t, db, "UPDATE droplets SET provider_resource_id='7' WHERE id=$1", droplet)
	c := app.Container{DB: db}
	owned := providers.Server{ID: "7", State: providers.ServerStateReady, Ready: true, Tags: []string{"managed-by-digital-ocean-bot", "dob-account-" + account}}
	inv := providers.Inventory{Servers: []providers.Server{owned, {ID: "8", State: providers.ServerStateReady, Tags: owned.Tags}}, ObservedAt: time.Now()}
	if e := c.SyncLinodeResources(context.Background(), account, inv); e != nil {
		t.Fatal(e)
	}
	var managed int
	if e := db.QueryRow("SELECT count(*) FROM resources WHERE account_id=$1 AND managed", account).Scan(&managed); e != nil || managed != 1 {
		t.Fatal(managed, e)
	}
	for _, bad := range []providers.Inventory{{}, {Servers: nil, ObservedAt: time.Now()}, {Servers: []providers.Server{owned, owned}, ObservedAt: time.Now()}, {Servers: []providers.Server{}, ObservedAt: time.Now().Add(-time.Hour)}} {
		if e := c.SyncLinodeResources(context.Background(), account, bad); e == nil {
			t.Fatal("bad inventory accepted")
		}
	}
	if e := db.QueryRow("SELECT count(*) FROM resources WHERE account_id=$1 AND state<>'deleted'", account).Scan(&managed); e != nil || managed != 2 {
		t.Fatal("bad snapshot erased resources", managed, e)
	}
	sqlMust(t, db, "INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed) VALUES(gen_random_uuid(),$1,'linode','9','server','ready',true)", other)
	inv.Servers = []providers.Server{}
	if e := c.SyncLinodeResources(context.Background(), account, inv); e != nil {
		t.Fatal(e)
	}
	if e := db.QueryRow("SELECT count(*) FROM resources WHERE account_id=$1 AND state<>'deleted'", account).Scan(&managed); e != nil || managed != 0 {
		t.Fatal(managed, e)
	}
	if e := db.QueryRow("SELECT count(*) FROM resources WHERE account_id=$1 AND state='ready'", other).Scan(&managed); e != nil || managed != 1 {
		t.Fatal("cross-account update", managed, e)
	}
}
func TestLinodeCapacityAndDashboardCount(t *testing.T) {
	db := adminTestDB(t)
	account := seedBuildAccount(t, db, "linode")
	s := Server{DB: db}
	if x := s.capacityEvidenceForAccount(account, "linode"); x.State != "UNKNOWN" {
		t.Fatal(x)
	}
	raw, _ := json.Marshal(providers.Observation{Account: providers.Account{Status: "active"}, Capacity: providers.Capacity{ComputeInUse: 3, LimitKnown: false, Source: "linode_api_usage_only"}})
	sqlMust(t, db, "INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical) VALUES(gen_random_uuid(),$1,'linode',2,'{}',$2)", account, raw)
	if x := s.capacityEvidenceForAccount(account, "linode"); x.State != "PROVEN_LOWER_BOUND" || x.LowerBound != 3 {
		t.Fatal(x)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.SetPathValue("id", account)
	s.accountCapacity(rr, req)
	var api map[string]any
	if e := json.Unmarshal(rr.Body.Bytes(), &api); e != nil || rr.Code != 200 || api["available"] != nil || api["server_limit"] != nil || api["limit_known"] != false || api["provider_servers"] != float64(3) {
		t.Fatal("unknown quota exposed as numeric capacity", rr.Code, api, e)
	}
	var body []byte
	if e := db.QueryRow(dashboardCountsSQL).Scan(&body); e != nil {
		t.Fatal(e)
	}
	var counts map[string]any
	if e := json.Unmarshal(body, &counts); e != nil {
		t.Fatal(e)
	}
	if counts["linode_accounts"] != float64(1) || counts["digitalocean_accounts"] != float64(0) {
		t.Fatal(counts)
	}
}
func TestLinodeBrowser(t *testing.T) {
	if os.Getenv("DOB_RUN_BROWSER_TEST") != "1" {
		t.Skip("browser opt-in")
	}
	registry := providers.NewRegistry()
	for _, f := range []providers.Factory{digitalocean.Factory{}, vultr.Factory{}, upcloud.Factory{}, linode.Factory{}} {
		if e := registry.Register(f); e != nil {
			t.Fatal(e)
		}
	}
	s := Server{Container: app.Container{Providers: registry}}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("../../web/static"))))
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "../../web/static/index.html") })
	mux.HandleFunc("GET /api/v1/providers", s.providersMetadata)
	mux.HandleFunc("GET /api/v1/accounts", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, []any{}) })
	mux.HandleFunc("GET /api/v1/proxies", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, []any{}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]bool{"ready": true}) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin})))
	}))
	defer server.Close()
	root := os.Getenv("DOB_UI_ARTIFACT_DIR")
	if root == "" {
		root = t.TempDir()
	}
	root, _ = filepath.Abs(root)
	os.MkdirAll(root, 0700)
	cmd := exec.Command("node", "../../web/tests/linode.cjs")
	cmd.Env = append(os.Environ(), "DOB_UI_BASE="+server.URL, "DOB_UI_ARTIFACT_DIR="+root)
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("%v %s", e, out)
	}
	t.Log(string(out))
}
