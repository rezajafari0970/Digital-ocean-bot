package adminapi

import (
	"context"
	"encoding/json"
	"time"

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
	mux.HandleFunc("POST /api/v1/accounts/preview", func(w http.ResponseWriter, r *http.Request) {
		var x previewAccount
		if json.NewDecoder(r.Body).Decode(&x) != nil {
			t.Error("preview request invalid")
			w.WriteHeader(400)
			return
		}
		time.Sleep(100 * time.Millisecond)
		discovery := providers.Discovery{Complete: true, Resolved: map[string]any{}}
		region := "eu-west-1"
		providerID := "123456789012:" + region
		if x.Provider == "ovh" {
			region = "GRA11"
			project := x.Selection["project_id"]
			providerID = "ovh-us:" + project
			discovery.Complete = project != ""
			discovery.Resolved["endpoint"] = "ovh-us"
			discovery.Fields = []providers.DiscoveryField{{Key: "project_id", Label: "Public Cloud project", Value: project, Options: []providers.DiscoveryOption{{Value: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Label: "Project A"}, {Value: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Label: "Project B"}}}}
			if project != "" {
				discovery.Resolved["project_id"] = project
			}
		} else {
			discovery.Resolved = map[string]any{"region": region, "subnet_id": "subnet-12345678", "managed_network": true}
			discovery.Message = "Dedicated automatic security group: IPv4 TCP/UDP 1–65535; default outbound rules."
			discovery.Fields = []providers.DiscoveryField{{Key: "region", Label: "AWS account region", Value: region, Options: []providers.DiscoveryOption{{Value: region, Label: region}}}}
		}
		writeJSON(w, 200, map[string]any{"provider": x.Provider, "onboarding": discovery, "account": providers.Account{ID: providerID}, "regions": []providers.Region{{ID: region, Name: region, Available: true}}, "plans": []providers.Plan{{ID: "test-plan", MemoryMB: 1024, CPU: 1, DiskGB: 20, Available: true}}, "images": []providers.Image{{ID: "ubuntu-fixture", Family: "ubuntu", Version: "24.04", Available: true}}, "defaults": map[string]any{}, "policy": map[string]any{}})
	})
	mux.HandleFunc("POST /api/v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		var x accountWrite
		if json.NewDecoder(r.Body).Decode(&x) != nil {
			t.Error("save body invalid")
			w.WriteHeader(400)
			return
		}
		var credential map[string]any
		if json.Unmarshal([]byte(x.Token), &credential) != nil {
			t.Error("credential not JSON")
			w.WriteHeader(400)
			return
		}
		if x.Provider == "aws" {
			if credential["region"] != "eu-west-1" || credential["subnet_id"] != "subnet-12345678" || credential["managed_network"] != true || credential["secret_access_key"] != "fixture-secret" {
				t.Error("AWS save lost pinned selection or secret")
				w.WriteHeader(400)
				return
			}
		} else if x.Provider == "ovh" {
			if credential["endpoint"] != "ovh-us" || credential["project_id"] != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" || credential["application_secret"] != "fixture-secret" {
				t.Error("OVH save lost pinned selection or secret")
				w.WriteHeader(400)
				return
			}
		} else {
			t.Error("wrong provider saved")
		}
		writeJSON(w, 201, map[string]string{"id": "fixture-account"})
	})
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
