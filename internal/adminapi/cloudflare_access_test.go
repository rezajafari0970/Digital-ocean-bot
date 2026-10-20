package adminapi

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflare"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"net/http/httptest"
	"testing"
)

type cfAccessFixture struct {
	*cfDriverFixture
	denied map[string]bool
	reads  int
}

func (d *cfAccessFixture) Records(ctx context.Context, z, n string) ([]cloudflare.Record, error) {
	d.reads++
	if d.denied[z] {
		return nil, &cloudflare.APIError{Status: 403, Operation: "dns_read"}
	}
	return d.cfDriverFixture.Records(ctx, z, n)
}

func TestCloudflareDNSAccessIsolationAndRecovery(t *testing.T) {
	db := adminTestDB(t)
	account, _, panel := cfSeed(t, db)
	ctx := context.Background()
	d := &cfAccessFixture{cfDriverFixture: &cfDriverFixture{records: map[string]cloudflare.Record{}}, denied: map[string]bool{"zone-a": true}}
	s := cloudflare.Service{DB: db, Factory: func(context.Context, string) (cloudflare.Driver, error) { return d, nil }}
	run := func() error {
		sqlMust(t, db, "UPDATE cloudflare_accounts SET next_attempt_at=now()")
		return s.RunOne(ctx)
	}
	for i := 0; i < 3; i++ {
		if e := run(); e == nil || e.Error() != "cloudflare_dns_read_permission_denied" {
			t.Fatal("permission was hidden", e)
		}
	}
	var count int
	if e := db.QueryRow("SELECT count(*) FROM cloudflare_records").Scan(&count); e != nil || count != 0 || d.reads != 1 || d.creates != 0 {
		t.Fatal("denial created intents or retried within cooldown", count, d.reads, d.creates, e)
	}
	if _, e := s.EnsureCertificate(ctx, account, panel, nil, provisioning.Target{}, nil); e == nil || e.Error() != "cloudflare_dns_read_permission_denied" {
		t.Fatal("certificate work ignored blocked DNS", e)
	}
	sqlMust(t, db, "INSERT INTO cloudflare_zones(account_id,id,name,status,enabled) VALUES($1,'zone-b','second.example.com','active',true)", account)
	_ = run()
	if d.creates != 1 {
		t.Fatal("healthy sibling blocked", d.creates)
	}
	var zone string
	if e := db.QueryRow("SELECT zone_id FROM cloudflare_records WHERE state='DNS_READY'").Scan(&zone); e != nil || zone != "zone-b" {
		t.Fatal(zone, e)
	}
	d.denied["zone-a"] = false
	sqlMust(t, db, "UPDATE cloudflare_zones SET dns_next_attempt_at=now() WHERE id='zone-a'")
	sqlMust(t, db, "UPDATE cloudflare_schedules SET next_batch_at=now()")
	if e := run(); e != nil {
		t.Fatal("recovery failed", e)
	}
	if d.creates != 2 {
		t.Fatal("recovery did not resume creation", d.creates)
	}
	var code string
	if e := db.QueryRow("SELECT error_code FROM cloudflare_accounts WHERE id=$1", account).Scan(&code); e != nil || code != "" {
		t.Fatal("stale account error", code, e)
	}
}

func TestCloudflareRetiresOnlyUnsentExpiredIntents(t *testing.T) {
	db := adminTestDB(t)
	account, profile, panel := cfSeed(t, db)
	ctx := context.Background()
	ids := map[string]string{}
	for _, state := range []string{"PREPARED", "SENT"} {
		id, _ := cloudflare.NewID()
		ids[state] = id
		sqlMust(t, db, "INSERT INTO cloudflare_records(id,account_id,zone_id,profile_id,panel_id,hostname,origin_ip,state,expires_at) VALUES($1,$2,'zone-a',$3,$4,$5,'192.0.2.5',$6,now()-interval '1 minute')", id, account, profile, panel, state+".example.com", state)
	}
	d := &cfAccessFixture{cfDriverFixture: &cfDriverFixture{records: map[string]cloudflare.Record{}}, denied: map[string]bool{"zone-a": true}}
	s := cloudflare.Service{DB: db, Factory: func(context.Context, string) (cloudflare.Driver, error) { return d, nil }}
	_ = s.RunOne(ctx)
	for original, id := range ids {
		var state string
		if e := db.QueryRow("SELECT state FROM cloudflare_records WHERE id=$1", id).Scan(&state); e != nil {
			t.Fatal(e)
		}
		want := "DELETED"
		if original == "SENT" {
			want = "SENT"
		}
		if state != want {
			t.Fatal(original, state)
		}
	}
	if d.creates != 0 || d.deletes != 0 || d.reads != 1 {
		t.Fatal("unsafe provider mutation or unbounded retry")
	}
}

func TestCloudflareStatusSeparatesIntentsAndDNS(t *testing.T) {
	db := adminTestDB(t)
	account, profile, panel := cfSeed(t, db)
	for _, state := range []string{"PREPARED", "SENT", "DNS_READY", "DELETING", "DELETED"} {
		id, _ := cloudflare.NewID()
		provider := ""
		if state == "DNS_READY" || state == "DELETING" {
			provider = "owned-" + id
		}
		sqlMust(t, db, "INSERT INTO cloudflare_records(id,account_id,zone_id,profile_id,panel_id,hostname,origin_ip,state,expires_at,provider_id) VALUES($1,$2,'zone-a',$3,$4,$5,'192.0.2.5',$6,now()+interval '1 hour',$7)", id, account, profile, panel, id+".example.com", state, provider)
	}
	s := Server{DB: db}
	w := httptest.NewRecorder()
	s.cloudflareStatus(w, httptest.NewRequest("GET", "/", nil))
	var out struct {
		Accounts []struct {
			Count   int `json:"record_count"`
			DNS     int `json:"dns_record_count"`
			Pending int `json:"pending_record_count"`
			Ready   int `json:"probe_ready_count"`
			Records []struct {
				Probe bool `json:"probe_ready"`
			}
		}
	}
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil || w.Code != 200 || len(out.Accounts) != 1 {
		t.Fatal("status", w.Code, e)
	}
	a := out.Accounts[0]
	if a.Count != 4 || a.DNS != 2 || a.Pending != 2 || a.Ready != 0 {
		t.Fatalf("misleading counts: %+v", a)
	}
	for _, r := range a.Records {
		if r.Probe {
			t.Fatal("probe enabled without published output")
		}
	}
}
