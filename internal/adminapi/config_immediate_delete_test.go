package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configplane"
)

func TestProfileDeleteImmediatelyRemovesLocalVisibility(t *testing.T) {
	db := adminTestDB(t)
	remoteCalls := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { remoteCalls++; w.WriteHeader(503) }))
	defer remote.Close()
	account, _, panel := seedPanel(t, db, remote.URL)
	sqlMust(t, db, "UPDATE config_control_authority SET unified=true WHERE singleton")
	api := &Server{DB: db}
	profiles := []configplane.Profile{}
	for i := 0; i < 4; i++ {
		code, p := profileSave(t, api, configplane.Profile{Name: fmt.Sprintf("Immediate %d", i), Enabled: true, Fleet: true, Classes: []string{"DIRECT"}, Count: 1, Spec: configdriver.Spec{Protocol: "vmess", Network: "tcp", Security: "none", Port: 18440 + i}}, "POST")
		if code != 200 {
			t.Fatalf("create %d: %d", i, code)
		}
		profiles = append(profiles, p)
	}
	for _, p := range profiles[:2] {
		raw, _ := json.Marshal(p.Spec)
		sqlMust(t, db, "INSERT INTO config_driver_listeners(profile_id,panel_id,managed_key,remote_id,credential_ref,applied_spec,state,stage,error_code,mutation_state,applied_revision) VALUES($1,$2,$1::text,$3,'owned-secret',$4,'BLOCKED','deleting','profile_delete_panel_unavailable','CONFIRMED',$5)", p.ID, panel, 100+p.Spec.Port, raw, p.Revision)
	}
	sqlMust(t, db, "UPDATE accounts SET provider_state='TOKEN_INVALID' WHERE id=$1", account)
	visible := func() map[string]bool {
		t.Helper()
		w := httptest.NewRecorder()
		api.getConfigProfiles(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != 200 {
			t.Fatalf("list: %d %s", w.Code, w.Body.String())
		}
		var body struct {
			Profiles []struct{ Profile configplane.Profile }
		}
		if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
			t.Fatal(e)
		}
		out := map[string]bool{}
		for _, p := range body.Profiles {
			out[p.Profile.ID] = true
		}
		return out
	}
	if len(visible()) != 4 {
		t.Fatal("fixture profiles missing")
	}
	remove := func() {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"revision": profiles[0].Revision})
		r := httptest.NewRequest("DELETE", "/", bytes.NewReader(body))
		r.SetPathValue("id", profiles[0].ID)
		w := httptest.NewRecorder()
		api.deleteConfigProfile(w, r)
		var receipt struct {
			ID, State string
			Cleanup   string `json:"cleanup_state"`
		}
		if json.Unmarshal(w.Body.Bytes(), &receipt) != nil || w.Code != 202 || receipt.ID != profiles[0].ID || receipt.State != "removed" || receipt.Cleanup != "pending" {
			t.Fatalf("receipt: %d %s", w.Code, w.Body.String())
		}
	}
	remove()
	remove() // A lost acknowledgement can be retried without a second intent.
	if v := visible(); len(v) != 3 || v[profiles[0].ID] {
		t.Fatal("removed profile remained visible", v)
	}
	before, err := configplane.Read(context.Background(), db, profiles[0].ID)
	if err != nil || before.Enabled || before.DeleteRequestedAt == nil || before.DeletedAt != nil || before.Revision != profiles[0].Revision+1 {
		t.Fatal("local removal lost durable unconfirmed cleanup", before, err)
	}
	// The reviewed batch removes its accepted item, preserves an independently
	// edited item and never expands to the profile created after the selection.
	sqlMust(t, db, "UPDATE config_driver_profiles SET revision=revision+1 WHERE id=$1", profiles[2].ID)
	body, _ := json.Marshal(map[string]any{"profiles": []profileDeleteItem{{profiles[1].ID, profiles[1].Revision}, {profiles[2].ID, profiles[2].Revision}}})
	w := httptest.NewRecorder()
	api.deleteConfigProfiles(w, httptest.NewRequest("POST", "/", bytes.NewReader(body)))
	var batch struct {
		Accepted, Conflicts int
		Results             []struct{ ID, State string }
	}
	if json.Unmarshal(w.Body.Bytes(), &batch) != nil || w.Code != 202 || batch.Accepted != 1 || batch.Conflicts != 1 || len(batch.Results) != 2 || batch.Results[0].State != "removed" || batch.Results[1].State != "revision_conflict" {
		t.Fatalf("batch: %d %s", w.Code, w.Body.String())
	}
	// A fresh server object has no client-side removal memory.
	api = &Server{DB: db}
	if v := visible(); len(v) != 2 || !v[profiles[2].ID] || !v[profiles[3].ID] {
		t.Fatal("removal depended on UI memory or changed an unrelated profile", v)
	}
	for _, p := range profiles[:2] {
		var state, ref string
		var remoteID int64
		var due bool
		if err := db.QueryRow("SELECT state,remote_id,credential_ref,next_attempt_at<=now() FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", p.ID, panel).Scan(&state, &remoteID, &ref, &due); err != nil || state != "BLOCKED" || remoteID != int64(100+p.Spec.Port) || ref != "owned-secret" || !due {
			t.Fatal("unconfirmed remote identity/cleanup discarded", state, remoteID, ref, due, err)
		}
		current, err := configplane.Read(context.Background(), db, p.ID)
		if err != nil || current.DeletedAt != nil || current.DeleteRequestedAt == nil || current.Enabled {
			t.Fatal("false remote completion", current, err)
		}
		if code, _ := profileSave(t, api, p, "PUT"); code != 409 {
			t.Fatal("stale update resurrected removed profile", code)
		}
	}
	if remoteCalls != 0 {
		t.Fatal("local removal waited on unavailable remote server", remoteCalls)
	}
}
