package adminapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

func TestForwardedPanelPortPreservesGateway(t *testing.T) {
	t.Setenv("DOB_PUBLIC_PANEL_PORT", "8443")
	db := adminTestDB(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("forwarded-fixture-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "INSERT INTO admin_users(id,username,password_hash,role) VALUES($1,'admin',$2,'admin')", perfUUID(), string(hash))
	authService := auth.Service{Store: auth.SQLStore{DB: db}}
	token, _, err := authService.Login(context.Background(), "admin", "forwarded-fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	control := &fakeUpdateController{state: updateState{Available: true, Status: "idle"}}
	s := Server{DB: db, Auth: authService, Updater: control}
	routes := s.Routes()
	request := func(method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://untrusted.invalid:9999/api/v1/panel-settings", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Forwarded-Port", "9999")
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, req)
		return w
	}
	var initial panelSettingsView
	w := request("GET", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &initial) != nil || initial.PublicPort != 8443 || initial.PublicPortEditable || initial.PortMode != "gateway" {
		t.Fatal(w.Code, w.Body.String())
	}
	body := func(port int, path string) string {
		return fmt.Sprintf(`{"listen_port":%d,"web_path":%q,"username":"admin","password":""}`, port, path)
	}
	w = request("PUT", body(9443, initial.WebPath))
	if w.Code != 409 || !strings.Contains(w.Body.String(), "public_port_requires_gateway_control") || len(control.actions) != 0 {
		t.Fatal("unsafe tunnel target accepted", w.Code, w.Body.String())
	}
	var saved int
	if err = db.QueryRow("SELECT listen_port FROM panel_settings").Scan(&saved); err != nil || saved != initial.ListenPort {
		t.Fatal("rejected change persisted", saved, err)
	}
	w = request("PUT", body(initial.ListenPort, "/custom-panel"))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"public_port":8443`) || !strings.Contains(w.Body.String(), `"restart_queued":true`) {
		t.Fatal("path change lost public gateway address", w.Code, w.Body.String())
	}
	t.Setenv("DOB_PUBLIC_PANEL_PORT", "invalid")
	if w = request("GET", ""); w.Code != 503 {
		t.Fatal("invalid ingress configuration ignored", w.Code)
	}
	if w = request("PUT", body(initial.ListenPort, "/admin")); w.Code != 503 {
		t.Fatal("invalid ingress configuration accepted mutation", w.Code)
	}
}
