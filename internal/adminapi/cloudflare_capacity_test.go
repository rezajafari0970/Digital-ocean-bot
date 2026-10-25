package adminapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflare"
)

func TestCloudflareFleetCapacityScopeAndOverlap(t *testing.T) {
	db := adminTestDB(t)
	account, profile, panel := cfSeed(t, db)
	sqlMust(t, db, "UPDATE accounts SET desired_server_count=10")
	sqlMust(t, db, "UPDATE config_driver_profiles SET spec=jsonb_set(jsonb_set(spec,'{cloudflare_records}','2'::jsonb),'{cloudflare_max_active}','6'::jsonb)")
	sqlMust(t, db, "UPDATE cloudflare_zones SET ssl_strict=true,checked_at=now(),dns_checked_at=now(),dns_record_quota=200,dns_record_usage=197,dns_quota_checked_at=now()")
	for _, n := range []string{"a", "b"} {
		sqlMust(t, db, `INSERT INTO cloudflare_records(id,account_id,zone_id,profile_id,panel_id,hostname,origin_ip,state,provider_id,activated_at,expires_at)
 VALUES(gen_random_uuid(),$1,'zone-a',$2,$3,$4,'192.0.2.5','DNS_READY',$4,now()-interval '1 minute',now()+interval '30 minutes')`, account, profile, panel, n+".example.com")
	}
	sqlMust(t, db, `INSERT INTO cloudflare_records(id,account_id,zone_id,profile_id,panel_id,hostname,origin_ip,state,expires_at)
 VALUES(gen_random_uuid(),$1,'zone-a',$2,$3,'pending.example.com','192.0.2.5','PREPARED',now()+interval '30 minutes')`, account, profile, panel)
	disabled, _, _ := seedPanel(t, db, "http://disabled.invalid")
	sqlMust(t, db, "UPDATE accounts SET enabled=false,desired_server_count=100 WHERE id=$1", disabled)
	// An unselected zone is never included in capacity, even with a large quota.
	sqlMust(t, db, `INSERT INTO cloudflare_zones(account_id,id,name,status,enabled,ssl_strict,checked_at,dns_checked_at,dns_record_quota,dns_quota_checked_at)
 VALUES($1,'unselected','other.example','active',false,true,now(),now(),5000,now())`, account)
	// Selected domains without strict TLS do not count as usable capacity.
	sqlMust(t, db, `INSERT INTO cloudflare_zones(account_id,id,name,status,enabled,ssl_strict,checked_at,dns_checked_at,dns_record_quota,dns_quota_checked_at)
 VALUES($1,'unsafe','unsafe.example','active',true,false,now(),now(),5000,now())`, account)
	svc := cloudflare.Service{DB: db}
	check := func() *cloudflare.FleetCapacity {
		t.Helper()
		got, err := svc.Capacity(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got[account] == nil {
			t.Fatal("missing account")
		}
		return got[account]
	}
	got := check()
	if got.Profiles != 1 || got.CurrentServers != 1 || got.PlannedServers != 10 || got.CurrentRequiredRecords != 6 || got.RequiredRecords != 60 {
		t.Fatalf("demand omitted targets or overlap: %+v", got)
	}
	if got.KnownRecordCapacity != 5 || got.AvailableNow != 2 || got.EligibleZones != 1 || got.BlockedZones != 1 || !got.Complete || got.Shortfall == nil || *got.Shortfall != 55 {
		t.Fatalf("capacity ignored unrelated usage, pending intents, or domain selection: %+v", got)
	}
	sqlMust(t, db, "UPDATE accounts SET provider_state='TOKEN_INVALID' WHERE id=(SELECT account_id FROM panel_instances WHERE id=$1)", panel)
	got = check()
	if got.CurrentServers != 0 || got.PlannedServers != 10 || got.RequiredRecords != 60 {
		t.Fatalf("authentication recovery lost target reservation: %+v", got)
	}
	// A targeted profile does not reserve the entire fleet.
	sqlMust(t, db, "UPDATE config_driver_profiles SET fleet=false,panel_ids=ARRAY[$2::uuid] WHERE id=$1", profile, panel)
	got = check()
	if got.PlannedServers != 1 || got.RequiredRecords != 6 {
		t.Fatalf("target scope expanded: %+v", got)
	}
	// A second profile consumes its own records; a different CF account is isolated.
	other, otherProfile, _ := cfSeed(t, db)
	sqlMust(t, db, "UPDATE config_driver_profiles SET fleet=false WHERE id=$1", otherProfile)
	got = check()
	all, err := svc.Capacity(context.Background())
	if err != nil || got.RequiredRecords != 6 || all[other].RequiredRecords != 0 {
		t.Fatal("account/profile isolation", err, all)
	}
	// Capacity is read-only and available with a one-connection pool. The API
	// must close its status cursor before starting this bounded diagnostic.
	db.SetMaxOpenConns(1)
	w := httptest.NewRecorder()
	(&Server{DB: db}).cloudflareStatus(w, httptest.NewRequest("GET", "/", nil))
	var body struct {
		Available bool                                `json:"capacity_available"`
		Capacity  map[string]cloudflare.FleetCapacity `json:"capacity"`
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != 200 || !body.Available || body.Capacity[account].RequiredRecords != 6 {
		t.Fatal("status lost capacity or exhausted its connection", w.Code, w.Body.String())
	}
	var records int
	if err = db.QueryRow("SELECT count(*) FROM cloudflare_records").Scan(&records); err != nil || records != 3 {
		t.Fatal("forecast mutated records", err, records)
	}
}

func TestCloudflareCapacityUnknownAndStaleNeverClaimsSufficient(t *testing.T) {
	db := adminTestDB(t)
	account, profile, _ := cfSeed(t, db)
	sqlMust(t, db, "UPDATE accounts SET desired_server_count=1")
	sqlMust(t, db, "UPDATE cloudflare_zones SET ssl_strict=true,checked_at=now(),dns_checked_at=now(),dns_record_quota=1000,dns_quota_checked_at=now()")
	check := func() *cloudflare.FleetCapacity {
		t.Helper()
		got, err := (cloudflare.Service{DB: db}).Capacity(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return got[account]
	}
	got := check()
	if got.KnownRecordCapacity != 500 || got.AvailableNow != 500 || got.RequiredRecords != 3 || got.Shortfall == nil || *got.Shortfall != 0 {
		t.Fatalf("worker zone ceiling or overlap changed: %+v", got)
	}
	for _, query := range []string{
		"UPDATE cloudflare_zones SET dns_quota_checked_at=now()-interval '3 minutes'",
		"UPDATE cloudflare_zones SET dns_quota_checked_at=now(),dns_record_quota=NULL",
		"UPDATE cloudflare_zones SET dns_record_quota=1000,dns_quota_error_code='cloudflare_access_denied'",
	} {
		sqlMust(t, db, query)
		got = check()
		if got.Complete || got.Shortfall != nil || got.UnknownZones != 1 || got.KnownRecordCapacity != 0 || got.AvailableNow != 0 {
			t.Fatalf("unknown quota reported sufficient: %+v", got)
		}
	}
	sqlMust(t, db, "UPDATE cloudflare_zones SET dns_quota_error_code='',dns_quota_checked_at=now()")
	// Bad legacy policy data must remain unknown, not a zero-demand success.
	sqlMust(t, db, "UPDATE config_driver_profiles SET spec=jsonb_set(spec,'{cloudflare_interval_seconds}','0'::jsonb) WHERE id=$1", profile)
	got = check()
	if got.Complete || got.Shortfall != nil || got.InvalidProfiles != 1 {
		t.Fatalf("invalid schedule concealed: %+v", got)
	}
}
