package clientops

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func TestBulkDeleteIdentityScopeAndMalformed(t *testing.T) {
	c := sanaei.Client{ID: "owned", Email: "owned-email", Enable: true}
	raw := func(id int, cs []sanaei.Client) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"id": id, "settings": map[string]any{"clients": cs}})
		return b
	}
	for _, snap := range [][]json.RawMessage{
		{raw(1, []sanaei.Client{{ID: "manual", Email: c.Email}})},
		{raw(1, []sanaei.Client{c}), raw(2, []sanaei.Client{c})},
		{json.RawMessage(`{"id":1,"settings":"broken"}`)},
		{json.RawMessage(`{"id":1,"settings":null}`)},
		{json.RawMessage(`{"id":1,"settings":{}}`)},
	} {
		if _, _, err := bulkDeleteObserved(snap, 1, []sanaei.Client{c}); err == nil {
			t.Fatal("unsafe snapshot accepted", string(snap[0]))
		}
	}
	got, absent, err := bulkDeleteObserved([]json.RawMessage{raw(1, []sanaei.Client{{ID: "manual", Email: "manual"}})}, 1, []sanaei.Client{c})
	if err != nil || len(got) != 0 || len(absent) != 1 {
		t.Fatal(got, absent, err)
	}
}

func TestBulkDeletePostgresOwnedPartialLostResponseAndRetry(t *testing.T) {
	db := bulkTestDB(t)
	j := Journal{DB: db}
	ctx := context.Background()
	p := bulkTestPayload(10)
	create, ok, err := j.ReserveBulk(ctx, "11111111-1111-4111-8111-111111111111", "33333333-3333-4333-8333-333333333333", 1, p)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE client_mutation_jobs SET state='SUCCEEDED' WHERE id=$1`, create); err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE bulk_user_ownership SET state='ACTIVE'`)
	bad := p
	bad.Clients = append([]sanaei.Client(nil), p.Clients...)
	bad.Clients[0].ID = "manual"
	if _, err = j.ReserveBulkDelete(ctx, "11111111-1111-4111-8111-111111111111", "33333333-3333-4333-8333-333333333333", 1, bad); !errors.Is(err, ErrClientConflict) {
		t.Fatal("manual admitted", err)
	}
	id, err := j.ReserveBulkDelete(ctx, "11111111-1111-4111-8111-111111111111", "33333333-3333-4333-8333-333333333333", 1, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE client_mutation_jobs SET payload='{}' WHERE id=$1`, id); err == nil {
		t.Fatal("mutable delete plan")
	}
	if _, ok, err = j.Claim(ctx); err != nil || ok {
		t.Fatal("closed main gate", ok, err)
	}
	db.Exec(`UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false`)
	job, ok, err := j.Claim(ctx)
	if err != nil || !ok {
		t.Fatal(err)
	}
	var mu sync.Mutex
	state := map[string]sanaei.Client{"manual": {ID: "manual-id", Email: "manual", Enable: true}}
	for _, c := range p.Clients {
		state[c.Email] = c
	}
	batches := [][]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "inbounds/list"):
			cs := []sanaei.Client{}
			for _, c := range state {
				cs = append(cs, c)
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": []any{map[string]any{"id": 1, "settings": map[string]any{"clients": cs}}}})
		case strings.HasSuffix(r.URL.Path, "clients/list"):
			records := []sanaei.GlobalClient{}
			for _, c := range state {
				records = append(records, sanaei.GlobalClient{UUID: c.ID, Email: c.Email, Enable: c.Enable, TotalGB: c.TotalGB, ExpiryTime: c.ExpiryTime, LimitHWID: c.LimitHWID, InboundIDs: []int64{1}})
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": records})
		case strings.Contains(r.URL.Path, "clients/get/"):
			c := state[strings.TrimPrefix(r.URL.Path, "/panel/api/clients/get/")]
			json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": map[string]any{"client": map[string]any{"uuid": c.ID, "email": c.Email}}})
		case strings.HasSuffix(r.URL.Path, "clients/bulkDel"):
			var request struct {
				Emails      []string `json:"emails"`
				KeepTraffic bool     `json:"keepTraffic"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			batches = append(batches, request.Emails)
			limit := len(request.Emails)
			if len(batches) == 1 {
				limit = 3
			}
			for _, email := range request.Emails[:limit] {
				if email == "manual" {
					t.Error("manual deletion")
				}
				delete(state, email)
			}
			if len(batches) == 1 {
				conn, _, _ := w.(http.Hijacker).Hijack()
				conn.Close()
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": map[string]any{"deleted": limit, "skipped": []any{}}})
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, err := sanaei.NewAPIClient(server.URL, sanaei.Credentials{Username: "test", Password: "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rt := &sanaei.PanelRuntime{Session: sanaei.NewPanelSession(client)}
	e := Executor{Journal: j}
	if err = e.executeBulkDelete(ctx, rt, job); err == nil {
		t.Fatal("partial delete reported success")
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM bulk_user_ownership WHERE state='DELETED'`).Scan(&n)
	if n != 3 {
		t.Fatal(n)
	}
	if err = j.Retry(ctx, id, "lost response", 0); err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE bulk_client_execution_gate SET remaining_batches=1`)
	job, ok, err = j.Claim(ctx)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err = e.executeBulkDelete(ctx, rt, job); err != nil {
		t.Fatal(err)
	}
	if err = e.executeBulkDelete(ctx, rt, job); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(batches) != 2 || len(batches[0]) != 10 || len(batches[1]) != 7 || len(state) != 1 || state["manual"].ID != "manual-id" {
		t.Fatal("bad recovery", batches, state)
	}
	db.QueryRow(`SELECT count(*) FROM bulk_user_ownership WHERE state='DELETED'`).Scan(&n)
	if n != 10 {
		t.Fatal(n)
	}
}

func TestBulkRecoveryCASRequiresClosedGatesAndObservedOwnership(t *testing.T) {
	db := bulkTestDB(t)
	j := Journal{DB: db}
	ctx := context.Background()
	p := bulkTestPayload(1)
	id, ok, err := j.ReserveBulk(ctx, "11111111-1111-4111-8111-111111111111", "33333333-3333-4333-8333-333333333333", 1, p)
	if err != nil || !ok {
		t.Fatal(err)
	}
	job, err := j.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := j.ClaimID(ctx, id); err != nil || claimed {
		t.Fatal("bulk ClaimID bypass", err)
	}
	if err = j.CompleteBulkRecovery(ctx, job, StateObsolete); err == nil {
		t.Fatal("open gate accepted")
	}
	if err = j.FailCloseGate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = j.CompleteBulkRecovery(ctx, job, StateObsolete); err == nil {
		t.Fatal("unobserved planned ownership accepted")
	}
	db.Exec(`UPDATE bulk_user_ownership SET state='ABORTED' WHERE mutation_job_id=$1`, id)
	if err = j.CompleteBulkRecovery(ctx, job, StateObsolete); err != nil {
		t.Fatal(err)
	}
	if err = j.CompleteBulkRecovery(ctx, job, StateObsolete); err == nil {
		t.Fatal("stale CAS accepted")
	}
}
