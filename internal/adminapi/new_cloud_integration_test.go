package adminapi

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewCloudInventoryOwnershipAndIncompleteSnapshot(t *testing.T) {
	for _, provider := range []string{"aws", "ovh", "kamatera"} {
		t.Run(provider, func(t *testing.T) {
			db := adminTestDB(t)
			account, droplet, _ := seedPanel(t, db, "http://linode.invalid")
			other, _, _ := seedPanel(t, db, "http://other.invalid")
			sqlMust(t, db, "UPDATE accounts SET provider=$3 WHERE id IN($1,$2)", account, other, provider)
			sqlMust(t, db, "UPDATE droplets SET provider_resource_id='7' WHERE id=$1", droplet)
			c := app.Container{DB: db}
			owned := providers.Server{ID: "7", State: providers.ServerStateReady, Ready: true, Tags: []string{"managed-by-digital-ocean-bot", "dob-account-" + account}}
			inv := providers.Inventory{Servers: []providers.Server{owned, {ID: "8", State: providers.ServerStateReady, Tags: owned.Tags}}, ObservedAt: time.Now()}
			if e := c.SyncCloudResources(context.Background(), account, provider, inv); e != nil {
				t.Fatal(e)
			}
			var managed int
			if e := db.QueryRow("SELECT count(*) FROM resources WHERE account_id=$1 AND managed", account).Scan(&managed); e != nil || managed != 1 {
				t.Fatal(managed, e)
			}
			for _, bad := range []providers.Inventory{{}, {Servers: nil, ObservedAt: time.Now()}, {Servers: []providers.Server{owned, owned}, ObservedAt: time.Now()}, {Servers: []providers.Server{}, ObservedAt: time.Now().Add(-time.Hour)}} {
				if e := c.SyncCloudResources(context.Background(), account, provider, bad); e == nil {
					t.Fatal("bad inventory accepted")
				}
			}
			if e := db.QueryRow("SELECT count(*) FROM resources WHERE account_id=$1 AND state<>'deleted'", account).Scan(&managed); e != nil || managed != 2 {
				t.Fatal("bad snapshot erased resources", managed, e)
			}
			sqlMust(t, db, "INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed) VALUES(gen_random_uuid(),$1,$2,'9','server','ready',true)", other, provider)
			inv.Servers = []providers.Server{}
			if e := c.SyncCloudResources(context.Background(), account, provider, inv); e != nil {
				t.Fatal(e)
			}
			if e := db.QueryRow("SELECT count(*) FROM resources WHERE account_id=$1 AND state<>'deleted'", account).Scan(&managed); e != nil || managed != 0 {
				t.Fatal(managed, e)
			}
			if e := db.QueryRow("SELECT count(*) FROM resources WHERE account_id=$1 AND state='ready'", other).Scan(&managed); e != nil || managed != 1 {
				t.Fatal("cross-account update", managed, e)
			}
		})
	}
}
func TestNewCloudCapacityAndDashboardCount(t *testing.T) {
	for _, provider := range []string{"aws", "ovh", "kamatera"} {
		t.Run(provider, func(t *testing.T) {
			db := adminTestDB(t)
			account := seedBuildAccount(t, db, provider)
			s := Server{DB: db}
			if x := s.capacityEvidenceForAccount(account, provider); x.State != "UNKNOWN" {
				t.Fatal(x)
			}
			raw, _ := json.Marshal(providers.Observation{Account: providers.Account{Status: "active"}, Capacity: providers.Capacity{ComputeInUse: 3, LimitKnown: false, Source: "linode_api_usage_only"}})
			sqlMust(t, db, "INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical) VALUES(gen_random_uuid(),$1,$3,2,'{}',$2)", account, raw, provider)
			if x := s.capacityEvidenceForAccount(account, provider); x.State != "PROVEN_LOWER_BOUND" || x.LowerBound != 3 {
				t.Fatal(x)
			}
			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			req.SetPathValue("id", account)
			s.accountCapacity(rr, req)
			var api map[string]any
			if e := json.Unmarshal(rr.Body.Bytes(), &api); e != nil || rr.Code != 200 || api["available"] != nil || api["server_limit"] != nil || api["limit_known"] != false || api["provider_servers"] != float64(3) {
				t.Fatal("unknown quota exposed as numeric capacity", rr.Code, api, e)
			}
			var body []byte
			if e := db.QueryRow(dashboardCountsSQL).Scan(&body); e != nil {
				t.Fatal(e)
			}
			var counts map[string]any
			if e := json.Unmarshal(body, &counts); e != nil {
				t.Fatal(e)
			}
			if counts[provider+"_accounts"] != float64(1) || counts["digitalocean_accounts"] != float64(0) {
				t.Fatal(counts)
			}
		})
	}
}
func TestNewCloudAbsenceEvidenceSurvivesRestart(t *testing.T) {
	db := adminTestDB(t)
	a, d, _ := seedPanel(t, db, "http://fixture.invalid")
	sqlMust(t, db, "UPDATE accounts SET provider='aws' WHERE id=$1", a)
	sqlMust(t, db, "UPDATE droplets SET provider_resource_id='eu-west-1:i-12345678',created_at=now()-interval '10 minutes' WHERE id=$1", d)
	verifier := providers.SQLAbsenceVerifier{DB: db, AccountID: a, Provider: "aws"}
	ctx := context.Background()
	id := "eu-west-1:i-12345678"
	if e := verifier.Confirm(ctx, id); !providers.IsClass(e, providers.ErrorUnavailable) {
		t.Fatal("first absence accepted", e)
	}
	sqlMust(t, db, "UPDATE provider_absence_evidence SET first_observed_at=now()-interval '3 minutes',last_observed_at=now()-interval '20 seconds',observations=2 WHERE account_id=$1", a)
	restarted := providers.SQLAbsenceVerifier{DB: db, AccountID: a, Provider: "aws"}
	if e := restarted.Confirm(ctx, id); e != nil {
		t.Fatal("stable absence rejected", e)
	}
	if e := restarted.Present(ctx, id); e != nil {
		t.Fatal(e)
	}
	if e := restarted.Confirm(ctx, id); !providers.IsClass(e, providers.ErrorUnavailable) {
		t.Fatal("presence did not reset absence", e)
	}
	if e := restarted.Confirm(ctx, "foreign"); !providers.IsClass(e, providers.ErrorUnavailable) {
		t.Fatal("unbound absence accepted", e)
	}
}
