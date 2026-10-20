package adminapi

import (
	"bytes"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configplane"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudflareScheduleAdmissionDetailAndPersistence(t *testing.T) {
	db := adminTestDB(t)
	account, _, _ := cfSeed(t, db)
	s := &Server{DB: db}
	p := configplane.Profile{Name: "Two records", Enabled: true, Fleet: true, Classes: []string{"DIRECT"}, Count: 1, Spec: configdriver.Spec{Protocol: "vless", Network: "ws", Security: "tls", Port: 2053, Path: "/cdn", CloudflareAccount: account, CloudflareRecords: 2, CloudflareInterval: 1800, CloudflareMinLifetime: 2700, CloudflareMaxLifetime: 3600, CloudflareMaxActive: 4}}
	raw, _ := json.Marshal(p)
	r := httptest.NewRequest("POST", "/", bytes.NewReader(raw))
	w := httptest.NewRecorder()
	s.saveConfigProfile(w, r)
	var response struct {
		Error, Detail, Field string
		RequiredActive       int `json:"required_active"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &response); e != nil {
		t.Fatal(e)
	}
	if w.Code != 400 || response.Error != "profile_cloudflare_schedule_invalid" || response.Field != "cloudflare_max_active" || response.RequiredActive != 6 || !strings.Contains(response.Detail, "at least 6") {
		t.Fatalf("missing actionable error: %d %+v", w.Code, response)
	}
	var n int
	if e := db.QueryRow("SELECT count(*) FROM config_driver_profiles").Scan(&n); e != nil || n != 1 {
		t.Fatal("invalid schedule persisted", n, e)
	}
	p.Spec.CloudflareMaxActive = 6
	code, saved := profileSave(t, s, p, "POST")
	if code != 200 || saved.Spec.CloudflareMaxActive != 6 || saved.Spec.CloudflareRecords != 2 {
		t.Fatalf("valid schedule rejected: %d", code)
	}
	var spec configdriver.Spec
	var stored []byte
	if e := db.QueryRow("SELECT spec FROM config_driver_profiles WHERE id=$1", saved.ID).Scan(&stored); e != nil {
		t.Fatal(e)
	}
	if e := json.Unmarshal(stored, &spec); e != nil || spec != saved.Spec {
		t.Fatal("saved schedule differs", e)
	}
}
