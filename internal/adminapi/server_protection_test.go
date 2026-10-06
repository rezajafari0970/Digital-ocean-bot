package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/serverprotection"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func protectionPolicy(t *testing.T, db *sql.DB, id string) serverprotection.Policy {
	t.Helper()
	p := serverprotection.Policy{PanelID: id}
	if e := db.QueryRow("SELECT desired_revision,desired_enabled FROM server_protection_nodes WHERE panel_id=$1", id).Scan(&p.Revision, &p.Enabled); e != nil {
		t.Fatal(e)
	}
	return p
}
func protectionReceipt(p serverprotection.Policy) serverprotection.Status {
	s := serverprotection.Status{Version: serverprotection.Version, Revision: p.Revision, Enabled: p.Enabled, AgentRunning: p.Enabled, NFTSupported: true, State: "READY", Ports: []int{443}, ObservedAt: time.Now()}
	if !p.Enabled {
		s.State = "DISABLED"
	}
	return s
}
func TestServerProtectionFleetRacesAndRollback(t *testing.T) {
	db := adminTestDB(t)
	_, dr, a := seedPanel(t, db, "http://a.test:2053")
	_, _, b := seedPanel(t, db, "http://b.test:2053")
	store := serverprotection.Store{DB: db}
	ctx := context.Background()
	if e := store.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM server_protection_nodes").Scan(&n)
	if n != 0 {
		t.Fatal("default off enrolled nodes")
	}
	request := serverprotection.Request{RequestID: perfUUID(), ExpectedRevision: 1, Enabled: true, Scope: "selected", PanelIDs: []string{a}}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := store.Change(ctx, request); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("duplicate lost-response retry", e)
		}
	}
	db.QueryRow("SELECT count(*) FROM server_protection_requests").Scan(&n)
	if n != 1 {
		t.Fatal("duplicate request ledger", n)
	}
	old := protectionPolicy(t, db, a)
	if e := store.Receipt(ctx, a, old, protectionReceipt(old), nil); e != nil {
		t.Fatal(e)
	}
	wrong := request
	wrong.Enabled = false
	if _, e := store.Change(ctx, wrong); !errors.Is(e, serverprotection.ErrConflict) {
		t.Fatal("request mutation not fenced", e)
	}
	_, e := store.Change(ctx, serverprotection.Request{RequestID: perfUUID(), ExpectedRevision: 2, Enabled: true, Scope: "fleet"})
	if e != nil {
		t.Fatal(e)
	}
	if e = store.Receipt(ctx, a, old, protectionReceipt(old), nil); !errors.Is(e, serverprotection.ErrConflict) {
		t.Fatal("stale receipt accepted", e)
	}
	_ = protectionPolicy(t, db, b)
	_, _, future := seedPanel(t, db, "http://future.test:2053")
	if e = store.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	_ = protectionPolicy(t, db, future)
	sqlMust(t, db, "UPDATE deployments SET state='FAILED' WHERE droplet_id=$1", dr)
	if e = store.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	if protectionPolicy(t, db, a).Enabled {
		t.Fatal("deployment withdrawal kept protection enabled")
	}
	sqlMust(t, db, "UPDATE deployments SET state='PANEL_COMPLETE' WHERE droplet_id=$1", dr)
	if e = store.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	before := protectionPolicy(t, db, a)
	sqlMust(t, db, "UPDATE droplets SET expires_at=now()+interval '1 second' WHERE id=$1", dr)
	if e = store.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	after := protectionPolicy(t, db, a)
	if after.Enabled || after.Revision <= before.Revision {
		t.Fatal("eligibility withdrawal re-used generation", before, after)
	}
	sqlMust(t, db, "UPDATE droplets SET expires_at=now()+interval '1 hour' WHERE id=$1", dr)
	store.Sync(ctx)
	back := protectionPolicy(t, db, a)
	if !back.Enabled || back.Revision <= after.Revision {
		t.Fatal("re-enrollment reused generation")
	}
	down, e := os.ReadFile("../../migrations/000157_server_protection.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(down)); e == nil {
		t.Fatal("enabled migration rollback allowed")
	}
	c, e := store.Change(ctx, serverprotection.Request{RequestID: perfUUID(), ExpectedRevision: 3, Enabled: false, Scope: "fleet"})
	if e != nil || c.Revision != 4 {
		t.Fatal(c, e)
	}
	if _, e = db.Exec(string(down)); e == nil {
		t.Fatal("unverified cleanup rollback allowed")
	}
	for _, id := range []string{a, b, future} {
		p := protectionPolicy(t, db, id)
		if p.Enabled {
			t.Fatal("disable missed assigned panel")
		}
		if e = store.Receipt(ctx, id, p, protectionReceipt(p), nil); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = db.Exec(string(down)); e != nil {
		t.Fatal("verified rollback failed", e)
	}
}
func TestServerProtectionOutputFreshnessAndAuthorization(t *testing.T) {
	db := adminTestDB(t)
	_, _, panel := seedPanel(t, db, "http://canary.test:2053")
	store := serverprotection.Store{DB: db}
	ctx := context.Background()
	if _, e := store.Change(ctx, serverprotection.Request{RequestID: perfUUID(), ExpectedRevision: 1, Enabled: true, Scope: "fleet"}); e != nil {
		t.Fatal(e)
	}
	p := protectionPolicy(t, db, panel)
	st := protectionReceipt(p)
	st.State = "CRITICAL"
	st.AdmissionBlocked = true
	if e := store.Receipt(ctx, panel, p, st, nil); e != nil {
		t.Fatal(e)
	}
	count := func() int {
		t.Helper()
		var n int
		if e := db.QueryRow("SELECT count(*) FROM panel_instances p WHERE true " + serverProtectionOutputPredicate).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	if count() != 0 {
		t.Fatal("fresh protected node published")
	}
	sqlMust(t, db, "UPDATE server_protection_nodes SET checked_at=now()-interval '20 seconds'")
	if count() != 0 {
		t.Fatal("stale blocked receipt republished node")
	}
	if e := store.Receipt(ctx, panel, p, serverprotection.Status{}, errors.New("SSH unavailable")); e != nil {
		t.Fatal(e)
	}
	if count() != 0 {
		t.Fatal("failed poll erased verified admission")
	}
	st.State = "READY"
	st.AdmissionBlocked = false
	st.XUIState = "active_xray_failed"
	if e := store.Receipt(ctx, panel, p, st, nil); e != nil {
		t.Fatal(e)
	}
	if count() != 0 {
		t.Fatal("active panel with failed Xray published")
	}
	st.XUIState = "active"
	if e := store.Receipt(ctx, panel, p, st, nil); e != nil {
		t.Fatal(e)
	}
	if count() != 1 {
		t.Fatal("healthy Xray receipt did not release output")
	}
	st.State = "CRITICAL"
	st.AdmissionBlocked = true
	if e := store.Receipt(ctx, panel, p, st, nil); e != nil {
		t.Fatal(e)
	}
	if _, e := store.Change(ctx, serverprotection.Request{RequestID: perfUUID(), ExpectedRevision: 2, Enabled: false, Scope: "fleet"}); e != nil {
		t.Fatal(e)
	}
	if count() != 0 {
		t.Fatal("disable intent is not proof of cleanup")
	}
	p = protectionPolicy(t, db, panel)
	if e := store.Receipt(ctx, panel, p, protectionReceipt(p), nil); e != nil {
		t.Fatal(e)
	}
	if count() != 1 {
		t.Fatal("verified cleanup did not release output")
	}
	server := Server{DB: db}
	r := httptest.NewRequest("GET", "/api/v1/server-protection", nil)
	w := httptest.NewRecorder()
	server.serverProtectionStatus(w, r)
	if w.Code != 403 {
		t.Fatal("non-admin accepted", w.Code)
	}
	r = r.WithContext(context.WithValue(ctx, principalKey{}, auth.Principal{Role: auth.Admin}))
	w = httptest.NewRecorder()
	server.serverProtectionStatus(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var data map[string]any
	if json.Unmarshal(w.Body.Bytes(), &data) != nil {
		t.Fatal("invalid status")
	}
	r = httptest.NewRequest("POST", "/api/v1/server-protection", strings.NewReader(`{"enabled":true,"unexpected":1}`)).WithContext(r.Context())
	w = httptest.NewRecorder()
	server.serverProtectionAction(w, r)
	if w.Code != 400 {
		t.Fatal("unknown field accepted", w.Code)
	}
}
