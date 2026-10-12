package adminapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/advancedreality"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
)

func engineRequest(method, path, body string, role auth.Role) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:12345"
	return r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: role}))
}
func TestAdvancedRealityDefaultOffAndAuthorization(t *testing.T) {
	t.Setenv("DOB_ADVANCED_REALITY_OBSERVE", "")
	s := Server{}
	w := httptest.NewRecorder()
	s.advancedRealityStatus(w, engineRequest("GET", "/api/v1/advanced-reality", "", auth.Admin))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"mode":"DISABLED"`) || strings.Contains(w.Body.String(), `"can_apply":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, role := range []auth.Role{auth.Viewer, auth.Operator, ""} {
		w = httptest.NewRecorder()
		s.advancedRealityStatus(w, engineRequest("GET", "/", "", role))
		if w.Code != 403 {
			t.Fatal(role, w.Code)
		}
	}
	w = httptest.NewRecorder()
	s.Routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/advanced-reality", nil))
	if w.Code != 401 {
		t.Fatal("anonymous", w.Code)
	}
	w = httptest.NewRecorder()
	s.advancedRealityValidate(w, engineRequest("POST", "/", "{}", auth.Admin))
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
}
func TestAdvancedRealityValidationRedactsAndNeverApplies(t *testing.T) {
	t.Setenv("DOB_ADVANCED_REALITY_OBSERVE", "1")
	s := Server{}
	b := `{"privateKey":"DO_NOT_ECHO","policy":{"levels":{"0":{"bufferSize":64}}}}`
	c := strings.Replace(b, `64`, `128`, 1)
	body, _ := json.Marshal(map[string]any{"baseline": json.RawMessage(b), "candidate": json.RawMessage(c), "core_version": "26.10.10"})
	w := httptest.NewRecorder()
	s.advancedRealityValidate(w, engineRequest("POST", "/", string(body), auth.Admin))
	if w.Code != 200 || strings.Contains(w.Body.String(), "DO_NOT_ECHO") || !strings.Contains(w.Body.String(), `"can_apply":false`) {
		t.Fatal(w.Code, w.Body.String())
	}
	body, _ = json.Marshal(map[string]any{"baseline": json.RawMessage(b), "candidate": json.RawMessage(strings.Replace(b, "DO_NOT_ECHO", "CHANGED_SECRET", 1)), "core_version": "26.10.10"})
	w = httptest.NewRecorder()
	s.advancedRealityValidate(w, engineRequest("POST", "/", string(body), auth.Admin))
	if w.Code != 422 || strings.Contains(w.Body.String(), "SECRET") || strings.Contains(w.Body.String(), "ECHO") {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, body := range []string{`{} {}`, `{"extra":1}`, strings.Repeat("x", 3*advancedreality.MaxConfigBytes)} {
		w = httptest.NewRecorder()
		s.advancedRealityValidate(w, engineRequest("POST", "/", body, auth.Admin))
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	r := engineRequest("POST", "/", string(body), auth.Admin)
	r.RemoteAddr = "192.0.2.1:12345"
	r.Header.Set("X-Forwarded-Proto", "https")
	w = httptest.NewRecorder()
	s.advancedRealityValidate(w, r)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "tls_or_loopback_required") {
		t.Fatal("untrusted forwarded TLS accepted")
	}
}
func TestAdvancedRealityInventoryUsesExistingSchema(t *testing.T) {
	db := adminTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b, e := advancedreality.Inventory(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	var v struct {
		CanApply bool  `json:"can_apply"`
		Panels   []any `json:"panels"`
	}
	if json.Unmarshal(b, &v) != nil || v.CanApply || v.Panels == nil || len(v.Panels) != 0 {
		t.Fatal(string(b))
	}
	const account = "810d360d-98e1-44ab-bd30-f83824601a49"
	const droplet = "810d360d-98e1-44ab-bd30-f83824601a50"
	const panel = "810d360d-98e1-44ab-bd30-f83824601a51"
	sqlMust(t, db, `INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'are-test','digitalocean','DO_NOT_EXPOSE_ACCOUNT_SECRET')`, account)
	sqlMust(t, db, `INSERT INTO droplets(id,account_id,state) VALUES($1,$2,'READY')`, droplet, account)
	sqlMust(t, db, `INSERT INTO panel_instances(id,account_id,droplet_id,driver,base_url,auth_secret_ref,version) VALUES($1,$2,$3,'sanaei','https://private-host.invalid/secret-path','DO_NOT_EXPOSE_PANEL_SECRET','3.10.0')`, panel, account, droplet)
	sqlMust(t, db, `INSERT INTO panel_inbound_inventory(panel_id,remote_id,protocol,port,security,transport,raw_hash,first_seen_at,last_seen_at,remark) VALUES($1,1,'vless',443,'reality','xhttp','hash',now(),now()+interval '1 hour','SECRET_CLIENT_NAME')`, panel)
	b, e = advancedreality.Inventory(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	for _, forbidden := range []string{"DO_NOT_EXPOSE", "private-host", "secret-path", "SECRET_CLIENT_NAME"} {
		if strings.Contains(string(b), forbidden) {
			t.Fatal("inventory leaked sensitive metadata")
		}
	}
	var full struct {
		Panels []struct {
			Core       *string `json:"core_version"`
			OSFresh    bool    `json:"os_metrics_fresh"`
			RouteFresh bool    `json:"route_fresh"`
			Inbounds   []struct {
				Fresh bool `json:"fresh"`
			} `json:"inbounds"`
		} `json:"panels"`
	}
	if json.Unmarshal(b, &full) != nil || len(full.Panels) != 1 || len(full.Panels[0].Inbounds) != 1 {
		t.Fatal("inventory fixture missing")
	}
	p := full.Panels[0]
	if p.Core != nil || p.OSFresh || p.RouteFresh || p.Inbounds[0].Fresh {
		t.Fatal("unknown/future observation reported healthy")
	}
}
