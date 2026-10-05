package adminapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/upcloud"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/vultr"
)

func TestUpCloudCleanupPostgresRestartAndConflict(t *testing.T) {
	db := adminTestDB(t)
	account, _, _ := seedPanel(t, db, "http://upcloud.invalid")
	sqlMust(t, db, "UPDATE accounts SET provider='upcloud' WHERE id=$1", account)
	ctx := context.Background()
	j := providers.SQLCleanupJournal{DB: db}
	manifest := providers.CleanupManifest{Identity: "deployment-one", StorageIDs: []string{"storage-two", "storage-one"}}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- j.Save(ctx, account, "server-one", manifest) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	// Recreate journal, as on worker restart; original ownership must survive.
	j = providers.SQLCleanupJournal{DB: db}
	saved, e := j.Load(ctx, account, "server-one")
	if e != nil || saved == nil || saved.Complete || len(saved.StorageIDs) != 2 {
		t.Fatal(saved, e)
	}
	if other, e := j.Load(ctx, perfUUID(), "server-one"); e != nil || other != nil {
		t.Fatal("tenant boundary", e)
	}
	bad := manifest
	bad.Identity = "other-deployment"
	if e = j.Save(ctx, account, "server-one", bad); e == nil {
		t.Fatal("manifest overwritten")
	}
	for _, id := range manifest.StorageIDs {
		sqlMust(t, db, "INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed) VALUES(gen_random_uuid(),$1,'upcloud',$2,'storage','online',true)", account, id)
	}
	sqlMust(t, db, "INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed) VALUES(gen_random_uuid(),$1,'upcloud','manual-disk','storage','online',false)", account)
	if e = j.Finish(ctx, account, "server-one"); e != nil {
		t.Fatal(e)
	}
	saved, e = j.Load(ctx, account, "server-one")
	if e != nil || !saved.Complete {
		t.Fatal(saved, e)
	}
	var managed, manual int
	db.QueryRow("SELECT count(*) FROM resources WHERE account_id=$1 AND type='storage' AND state='deleted'", account).Scan(&managed)
	db.QueryRow("SELECT count(*) FROM resources WHERE account_id=$1 AND provider_resource_id='manual-disk' AND state='online'", account).Scan(&manual)
	if managed != 2 || manual != 1 {
		t.Fatal("storage completion mismatch", managed, manual)
	}
}
func TestUpCloudCapacityEvidenceAndMigrationGuard(t *testing.T) {
	db := adminTestDB(t)
	account, _, _ := seedPanel(t, db, "http://upcloud.invalid")
	sqlMust(t, db, "UPDATE accounts SET provider='upcloud' WHERE id=$1", account)
	s := Server{DB: db}
	if got := s.capacityEvidenceForAccount(account, "upcloud"); got.State != "UNKNOWN" {
		t.Fatal(got)
	}
	raw, _ := json.Marshal(providers.Observation{Capacity: providers.Capacity{LimitKnown: true, ComputeLimit: 7, ComputeInUse: 2, Source: "upcloud_resource_budget"}})
	sqlMust(t, db, "INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical) VALUES(gen_random_uuid(),$1,'upcloud',2,'{}',$2)", account, raw)
	if got := s.capacityEvidenceForAccount(account, "upcloud"); got.State != "RESOURCE_BUDGET" {
		t.Fatal(got)
	}
	down, e := os.ReadFile("../../migrations/000153_upcloud_cleanup.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(down)); e == nil {
		t.Fatal("unsafe downgrade accepted")
	}
	if _, e = db.Exec("SELECT 1 FROM provider_cleanup_manifests LIMIT 1"); e != nil {
		t.Fatal("downgrade modified schema", e)
	}
}
func TestUpCloudInventoryPersistsServersAndOwnedStorage(t *testing.T) {
	db := adminTestDB(t)
	account, _, _ := seedPanel(t, db, "http://upcloud.invalid")
	sqlMust(t, db, "UPDATE accounts SET provider='upcloud' WHERE id=$1", account)
	c := app.Container{DB: db}
	inv := providers.Inventory{Servers: []providers.Server{{ID: "up-server", Name: "fixture", State: providers.ServerStateReady, Ready: true, PrimaryIPv4: "203.0.113.1"}}, Resources: []providers.Resource{{ID: "owned-storage", Type: "storage", State: "online", Managed: true}}}
	if e := c.SyncUpCloudResources(context.Background(), account, inv); e != nil {
		t.Fatal(e)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM resources WHERE account_id=$1 AND provider='upcloud' AND state<>'deleted'", account).Scan(&n)
	if n != 2 {
		t.Fatal(n)
	}
	if e := c.SyncUpCloudResources(context.Background(), account, providers.Inventory{}); e != nil {
		t.Fatal(e)
	}
	db.QueryRow("SELECT count(*) FROM resources WHERE account_id=$1 AND provider='upcloud' AND state<>'deleted'", account).Scan(&n)
	if n != 0 {
		t.Fatal(n)
	}
}
func TestUpCloudBrowser(t *testing.T) {
	if os.Getenv("DOB_RUN_BROWSER_TEST") != "1" {
		t.Skip("browser opt-in")
	}
	registry := providers.NewRegistry()
	for _, f := range []providers.Factory{digitalocean.Factory{}, vultr.Factory{}, upcloud.Factory{}} {
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
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin}))
		mux.ServeHTTP(w, r)
	}))
	defer server.Close()
	cmd := exec.Command("node", "../../web/tests/upcloud.cjs")
	root := os.Getenv("DOB_UI_ARTIFACT_DIR")
	if root == "" {
		root = t.TempDir()
	}
	root, _ = filepath.Abs(root)
	cmd.Env = append(os.Environ(), "DOB_UI_BASE="+server.URL, "DOB_UI_ARTIFACT_DIR="+root)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, out)
	} else {
		t.Log(string(out))
	}
}
