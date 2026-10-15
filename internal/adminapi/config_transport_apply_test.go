package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"net/http/httptest"
	"testing"
)

func TestTransportApplySaveIsAtomicAndScopesEligibleListeners(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	s := Server{DB: db}
	_, _, panel := seedPanel(t, db, "http://fixture")
	_, _, disabled := seedPanel(t, db, "http://disabled")
	const key = "dob:global-reality:00443"
	for _, p := range []string{panel, disabled} {
		if _, e := transportprofile.Resolve(ctx, db, p, key, "tcp"); e != nil {
			t.Fatal(e)
		}
		sqlMust(t, db, "INSERT INTO panel_inbound_inventory(panel_id,remote_id,remark,protocol,transport,security,port,present,enabled,raw_hash,first_seen_at,last_seen_at) VALUES($1,1,$2,'vless','tcp','reality',443,true,true,'fixture',now(),now())", p, key)
	}
	sqlMust(t, db, "UPDATE panel_instances SET enabled=false WHERE id=$1", disabled)
	c := transportprofile.Recommended()
	c.Host = "aparat.com"
	save := func(rev int64, c transportprofile.Config) int {
		raw, _ := json.Marshal(transportRequest{Policy: transportprofile.Policy{Revision: rev, Config: c}, ApplyExisting: true})
		w := httptest.NewRecorder()
		s.putConfigTransport(w, httptest.NewRequest("PUT", "/", bytes.NewReader(raw)))
		return w.Code
	}
	if code := save(1, c); code != 200 {
		t.Fatal("save+queue", code)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM reality_transport_apply_jobs WHERE panel_id=$1 AND state='PENDING'", panel).Scan(&n)
	if n != 1 {
		t.Fatal("missing queued apply")
	}
	db.QueryRow("SELECT count(*) FROM reality_transport_apply_jobs WHERE panel_id=$1", disabled).Scan(&n)
	if n != 0 {
		t.Fatal("disabled panel queued")
	}
	pin, e := transportprofile.Resolve(ctx, db, panel, key, "tcp")
	if e != nil || pin.NetworkName() != "tcp" {
		t.Fatal("queue changed active pin", e)
	}
	c.Host = "later.example.org"
	if code := save(2, c); code != 409 {
		t.Fatal("overlap accepted", code)
	}
	p, e := transportprofile.Read(ctx, db)
	if e != nil || p.Revision != 2 || p.Config.Host != "aparat.com" {
		t.Fatal("conflict partially saved policy", e)
	}
	if code := save(1, c); code != 409 {
		t.Fatal("stale write", code)
	}
	var id string
	db.QueryRow("SELECT id::text FROM reality_transport_apply_jobs WHERE panel_id=$1", panel).Scan(&id)
	sqlMust(t, db, "UPDATE reality_transport_apply_jobs SET state='BLOCKED',before_secret_ref='encrypted-backup',restart_sent_at=now(),attempts=5 WHERE id=$1", id)
	req := httptest.NewRequest("POST", "/", nil)
	req.SetPathValue("id", id)
	w := httptest.NewRecorder()
	s.retryConfigTransport(w, req)
	if w.Code != 200 {
		t.Fatal("retry", w.Code)
	}
	var state string
	var attempts int
	var sent bool
	db.QueryRow("SELECT state,attempts,restart_sent_at IS NOT NULL FROM reality_transport_apply_jobs WHERE id=$1", id).Scan(&state, &attempts, &sent)
	if state != "RUNNING" || attempts != 0 || !sent {
		t.Fatal("retry released native/output fence", state, attempts)
	}
	// A recovery child inherits native uncertainty before its own backup exists.
	sqlMust(t, db, "UPDATE reality_transport_apply_jobs SET state='BLOCKED',before_secret_ref='',source_config=target_config WHERE id=$1", id)
	req = httptest.NewRequest("POST", "/", nil)
	req.SetPathValue("id", id)
	w = httptest.NewRecorder()
	s.retryConfigTransport(w, req)
	db.QueryRow("SELECT state FROM reality_transport_apply_jobs WHERE id=$1", id).Scan(&state)
	if w.Code != 200 || state != "RUNNING" {
		t.Fatal("inherited native uncertainty lost output fence", w.Code, state)
	}
	req = httptest.NewRequest("POST", "/", nil)
	req.SetPathValue("id", "invalid")
	w = httptest.NewRecorder()
	s.rollbackConfigTransport(w, req)
	if w.Code != 400 {
		t.Fatal("invalid rollback ID", w.Code)
	}
}
