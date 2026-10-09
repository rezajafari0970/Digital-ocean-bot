package residentialsync

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestManagedSOCKSProvisioningPostgres(t *testing.T) {
	t.Setenv("DOB_RESIDENTIAL_POOL_PANELS", "")
	db := trialTestDB(t)
	ctx := context.Background()
	for _, key := range []string{"DOB_UPCLOUD_RELAY_PANELS", "DOB_RESIDENTIAL_ALLOWLIST_PANELS", "DOB_RESIDENTIAL_CLIENT_PATHS_PANELS", "DOB_RESIDENTIAL_ADS_ONLY_PANELS", "DOB_RESIDENTIAL_HARDENING_PANELS"} {
		t.Setenv(key, "all")
	}
	_, _, _ = relayDBPanel(t, db, "upcloud", "198.51.100.9", true)
	account, panel, _ := relayDBPanel(t, db, "vultr", "203.0.113.7", false)
	sqlMustTrial(t, db, "INSERT INTO panel_routing_state(panel_id,state,pool_enabled,verified_at)VALUES($1,'APPLIED',true,now())", panel)
	store, e := secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
	if e != nil {
		t.Fatal(e)
	}
	conflict := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var obj any
		switch r.URL.Path {
		case "/panel/api/inbounds/list":
			ins := []any{map[string]any{"id": 1, "tag": "public", "port": 443, "protocol": "vless"}}
			if conflict {
				ins = append(ins, map[string]any{"id": 2, "tag": "foreign", "port": 8080, "protocol": "http"})
			}
			obj = ins
		case "/panel/api/xray/":
			obj = map[string]any{"xraySetting": map[string]any{"inbounds": []any{map[string]any{"tag": "api", "port": 62789}}}, "outboundTestUrl": ""}
		default:
			t.Errorf("unexpected API mutation %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 400)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": obj})
	}))
	defer server.Close()
	client, e := sanaei.NewAPIClient(server.URL, sanaei.Credentials{Username: "fixture", Password: "fixture"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	rt := &sanaei.PanelRuntime{PanelID: panel, AccountID: account, Session: sanaei.NewPanelSession(client)}
	svc := Service{DB: db, Secrets: store}
	if e = svc.prepareRelay(ctx, panel, rt); e != nil {
		t.Fatal(e)
	}
	in, e := svc.loadRelayInlet(ctx, panel)
	if e != nil || in == nil || in.Port != 8080 || len(in.Sources) != 1 {
		t.Fatal("inlet provisioning failed", e)
	}
	ref := in.SecretRef
	if e = svc.prepareRelay(ctx, panel, rt); e != nil {
		t.Fatal(e)
	}
	in, e = svc.loadRelayInlet(ctx, panel)
	if e != nil || in.SecretRef != ref {
		t.Fatal("retry rotated credential", e)
	}
	conflict = true
	if e = svc.prepareRelay(ctx, panel, rt); e != nil {
		t.Fatal(e)
	}
	in, e = svc.loadRelayInlet(ctx, panel)
	if e != nil || in.Port != 80 {
		t.Fatal("occupied port not preserved", e)
	}
	t.Setenv("DOB_UPCLOUD_RELAY_PANELS", "none")
	if e = svc.prepareRelay(ctx, panel, rt); e != nil {
		t.Fatal(e)
	}
	in, e = svc.loadRelayInlet(ctx, panel)
	if e != nil || in != nil {
		t.Fatal("disabled donor remains")
	}
}
