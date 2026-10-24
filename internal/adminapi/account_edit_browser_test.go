package adminapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAccountEditBrowser(t *testing.T) {
	if os.Getenv("DOB_RUN_BROWSER_TEST") != "1" {
		t.Skip("browser opt-in")
	}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("../../web/static"))))
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "../../web/static/index.html") })
	mux.HandleFunc("GET /api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"id": "fixture", "role": "admin"})
	})
	providers := []string{"aws", "ovh", "vultr", "digitalocean", "linode", "upcloud"}
	accounts := []map[string]any{}
	for _, p := range providers {
		accounts = append(accounts, map[string]any{"id": p, "provider": p, "name": p + " fixture", "enabled": true, "provider_state": "ACTIVE", "regions": []string{"r1"}, "sizes": []string{"plan-1"}, "images": []string{"ubuntu-1"}, "image": "ubuntu-1", "proxy_id": "saved-proxy", "proxy_ids": []string{"saved-proxy"}, "desired_server_count": 10, "max_concurrent": 3, "lifetime_min_seconds": 96000, "lifetime_max_seconds": 99000, "build_spacing_minutes": 15, "build_spacing_max_minutes": 260})
	}
	mux.HandleFunc("GET /api/v1/accounts", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, accounts) })
	mux.HandleFunc("GET /api/v1/proxies", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, []map[string]string{{"id": "saved-proxy", "name": "Saved proxy", "status": "healthy"}, {"id": "other-proxy", "name": "Other proxy", "status": "healthy"}})
	})
	mux.HandleFunc("GET /api/v1/accounts/{id}/options", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"provider": r.PathValue("id"), "regions": []map[string]any{{"ID": "r1", "Name": "Region one", "Available": true}, {"ID": "r2", "Name": "Region two", "Available": true}},
			"plans":  []map[string]any{{"ID": "plan-1", "Available": true, "MemoryMB": 2048, "CPU": 2, "DiskGB": 20, "AvailableRegions": []string{"r1"}}, {"ID": "plan-2", "Available": true, "MemoryMB": 4096, "CPU": 2, "DiskGB": 30, "AvailableRegions": []string{"r2"}}},
			"images": []map[string]any{{"ID": "ubuntu-1", "Family": "ubuntu", "Version": "26.04", "Available": true, "AvailableRegions": []string{"r1"}}, {"ID": "ubuntu-2", "Family": "ubuntu", "Version": "24.04", "Available": true, "AvailableRegions": []string{"r2"}}}})
	})
	mux.HandleFunc("PUT /api/v1/accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		for field, want := range map[string]any{"sizes": []any{"plan-1"}, "images": []any{"ubuntu-1"}, "regions": []any{"r1"}, "proxy_ids": []any{"saved-proxy"}, "token": "", "desired_server_count": float64(10), "max_concurrent": float64(3), "build_spacing_minutes": float64(15), "build_spacing_max_minutes": float64(260)} {
			if !reflect.DeepEqual(body[field], want) {
				t.Errorf("%s changed %s: got %#v want %#v", r.PathValue("id"), field, body[field], want)
				w.WriteHeader(400)
				return
			}
		}
		writeJSON(w, 200, map[string]any{"saved": true})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]bool{"ready": true}) })
	settings := map[string]any{"listen_port": 28080, "public_port": 8443, "public_port_editable": false, "port_mode": "gateway", "web_path": "/admin", "username": "admin"}
	mux.HandleFunc("GET /api/v1/panel-settings", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, settings) })
	mux.HandleFunc("GET /api/v1/project-update", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"available": true, "status": "idle"})
	})
	mux.HandleFunc("PUT /api/v1/panel-settings", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["listen_port"] != float64(28080) || body["web_path"] != "/admin" {
			t.Error("settings used stale account callback or wrong port")
			w.WriteHeader(400)
			return
		}
		writeJSON(w, 200, map[string]any{"settings": settings, "saved": true, "restart_required": false})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	root := os.Getenv("DOB_UI_ARTIFACT_DIR")
	if root == "" {
		root = t.TempDir()
	}
	root, _ = filepath.Abs(root)
	os.MkdirAll(root, 0700)
	cmd := exec.Command("node", "../../web/tests/account-edit.cjs")
	cmd.Env = append(os.Environ(), "DOB_UI_BASE="+srv.URL, "DOB_UI_ARTIFACT_DIR="+root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	} else {
		t.Log(string(out))
	}
}
