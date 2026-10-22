package adminapi

import (
	"context"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/aws"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/linode"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ovh"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/upcloud"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/vultr"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNewCloudProviderBrowser(t *testing.T) {
	if os.Getenv("DOB_RUN_BROWSER_TEST") != "1" {
		t.Skip("browser opt-in")
	}
	registry := providers.NewRegistry()
	for _, f := range []providers.Factory{digitalocean.Factory{}, vultr.Factory{}, upcloud.Factory{}, linode.Factory{}, aws.Factory{}, ovh.Factory{}} {
		if e := registry.Register(f); e != nil {
			t.Fatal(e)
		}
	}
	s := Server{Container: app.Container{Providers: registry}}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("../../web/static"))))
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "../../web/static/index.html") })
	mux.HandleFunc("GET /api/v1/providers", s.providersMetadata)
	// Match the authenticated panel bootstrap before exercising visible account controls.
	mux.HandleFunc("GET /api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": "isolated-browser-fixture", "role": "admin"})
	})
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
	cmd := exec.Command("node", "../../web/tests/new-cloud-providers.cjs")
	cmd.Env = append(os.Environ(), "DOB_UI_BASE="+server.URL, "DOB_UI_ARTIFACT_DIR="+root)
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("%v %s", e, out)
	}
	t.Log(string(out))
}
