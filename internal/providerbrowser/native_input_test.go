package providerbrowser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Reuses the production worker inside the existing disposable native sandbox.
// Only the route/catalog/auth fixture is synthetic; input and provider DOM are real.
func nativePageInput(t *testing.T, peer *Peer) {
	t.Helper()
	root, err := filepath.Abs("../../web")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if strings.HasPrefix(r.URL.Path, "/static/") {
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src data:; connect-src 'self'; form-action 'self'; base-uri 'none'")
			http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(root, "static")))).ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer input-fixture" {
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/proxies" {
			json.NewEncoder(w).Encode([]any{map[string]any{"id": "fixture", "name": "Fixture proxy", "status": "healthy"}})
			return
		}
		if r.URL.Path == "/api/v1/proxy-drivers" {
			json.NewEncoder(w).Encode(map[string]any{"country_options": []any{map[string]string{"code": "DE", "label": "Germany"}}})
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(204)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if r.URL.Path == "/api/v1/provider-browser/sessions" {
			var a Action
			if json.NewDecoder(r.Body).Decode(&a) != nil {
				http.Error(w, "bad request", 400)
				return
			}
			a.Kind = "resize"
			if _, err := peer.Call(ctx, a); err != nil {
				http.Error(w, "resize failed", 500)
				return
			}
			frame, err := peer.Call(ctx, Action{Kind: "navigate", URL: "https://fixture.invalid/input"})
			if err != nil {
				http.Error(w, "navigate failed", 500)
				return
			}
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]any{"id": "native-input", "proxy_id": "fixture", "country": "DE", "exit_ip": "192.0.2.1", "expires_at": time.Now().Add(5 * time.Minute), "frame": frame})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/action") {
			var a Action
			if json.NewDecoder(r.Body).Decode(&a) != nil {
				http.Error(w, "bad request", 400)
				return
			}
			// Realistic latency that reproduced split-touch long presses.
			time.Sleep(120 * time.Millisecond)
			frame, err := peer.Call(ctx, a)
			if err != nil {
				t.Logf("native UI action %s failed: %v", a.Kind, err)
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]string{"error": "action failed"})
				return
			}
			if a.Kind != "frame" {
				if f := frame.Focused; f != nil {
					t.Logf("native UI %s key=%s focus=true multiline=%t password=%t selection=%d:%d", a.Kind, a.Key, f.Multiline, f.Password, f.Start, f.End)
				} else {
					t.Logf("native UI %s key=%s focus=false", a.Kind, a.Key)
				}
			}
			json.NewEncoder(w).Encode(frame)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", filepath.Join(root, "tests/provider-browser-native-input.cjs"))
	command.Env = append(os.Environ(), "DOB_NATIVE_INPUT_URL="+server.URL)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("native page input: %v\n%s", err, output)
	}
	t.Log(string(output))
}

const nativeInputFixture = `<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1"><title>Native input fixture</title>
<style>body{margin:0;font:16px system-ui}input,textarea{position:absolute;left:24px;width:342px;height:44px;font:16px system-ui;box-sizing:border-box}#field{top:80px}#password{top:140px}#area{top:200px;height:70px}#commit{position:absolute;top:290px;left:24px;width:342px;height:40px}#consent{position:fixed;z-index:10;inset:0 0 auto;height:280px;background:#eee}#dismiss{position:absolute;top:190px;left:24px;width:342px;height:40px}</style>
<input id="field" aria-label="Text field" oninput="document.title='input:'+this.value+'|'+(event.isTrusted?'trusted':'untrusted')">
<input id="password" type="password" aria-label="Password" oninput="document.title='password-edited'">
<textarea id="area" aria-label="Multiline" oninput="document.title='multiline:'+this.value"></textarea>
<button id="commit" onclick="document.title='commit:'+document.querySelector('#field').value">Commit</button>
<div id="consent"><h1>Cookie choices</h1><button id="dismiss" onclick="this.parentElement.remove();document.title='consent-dismissed'">Continue without optional cookies</button></div>`
