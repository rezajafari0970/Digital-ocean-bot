package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestAccountDashboardCanonicalInventoryAndFreshness(t *testing.T) {
	db := adminTestDB(t)
	s := Server{DB: db}
	for _, provider := range []string{"digitalocean", "vultr", "linode", "upcloud"} {
		t.Run(provider, func(t *testing.T) {
			id := seedBuildAccount(t, db, provider)
			read := func() accountDashboard {
				t.Helper()
				w := httptest.NewRecorder()
				r := httptest.NewRequest("GET", "/", nil)
				r.SetPathValue("id", id)
				s.accountDashboard(w, r)
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
				var d accountDashboard
				if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
					t.Fatal(err)
				}
				return d
			}
			d := read()
			if d.Resources["total"] != nil || d.Resources["data_available"] != false {
				t.Fatal("missing observation invented zero", d.Resources)
			}
			sqlMust(t, db, `INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES(gen_random_uuid(),$1,'owned','READY'),(gen_random_uuid(),$1,'retired','DELETED')`, id)
			// Legacy aliases are deliberately duplicated and contradictory.
			sqlMust(t, db, `INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed) VALUES(gen_random_uuid(),$1,$2,'owned','droplet','active',true),(gen_random_uuid(),$1,$2,'owned','server','ready',true),(gen_random_uuid(),$1,$2,'ghost','server','ready',true)`, id, provider)
			obs := providers.Observation{Account: providers.Account{Status: "active"}, Inventory: providers.Inventory{Servers: []providers.Server{
				{ID: "owned", State: providers.ServerStateReady}, {ID: "owned", State: providers.ServerStateReady},
				{ID: "foreign", State: providers.ServerStateStopped}, {ID: "retired", State: providers.ServerStateReady},
			}}}
			raw, _ := json.Marshal(obs)
			sqlMust(t, db, `INSERT INTO provider_snapshots(id,account_id,provider,data,canonical) VALUES(gen_random_uuid(),$1,$2,'{}',$3)`, id, provider, raw)
			d = read()
			if d.Resources["total"] != float64(3) || d.Resources["managed"] != float64(1) || d.Resources["unmanaged"] != float64(2) || d.Resources["active"] != float64(2) || len(d.Droplets) != 3 {
				t.Fatal(d.Resources, d.Droplets)
			}
			sqlMust(t, db, `UPDATE accounts SET provider_inventory_interval_seconds=300 WHERE id=$1`, id)
			sqlMust(t, db, `UPDATE provider_snapshots SET created_at=now()-interval '3 minutes' WHERE account_id=$1`, id)
			d = read()
			if d.Capacity["data_status"] != "scheduled" || d.Capacity["stale_after_seconds"] != float64(330) || d.Capacity["fresh_for_seconds"] != float64(120) {
				t.Fatal(d.Capacity)
			}
			sqlMust(t, db, `UPDATE provider_snapshots SET created_at=now()-interval '6 minutes' WHERE account_id=$1`, id)
			d = read()
			if d.Capacity["data_status"] != "expired" || d.Resources["total"] != float64(3) {
				t.Fatal(d.Capacity, d.Resources)
			}
			sqlMust(t, db, `UPDATE provider_snapshots SET created_at=now()+interval '1 minute' WHERE account_id=$1`, id)
			d = read()
			if d.Capacity["data_status"] == "fresh" || d.Capacity["data_status"] == "scheduled" {
				t.Fatal("future snapshot treated as fresh", d.Capacity)
			}
			sqlMust(t, db, `UPDATE provider_snapshots SET created_at=now(),canonical=jsonb_set(canonical,'{Inventory,Servers}','[]') WHERE account_id=$1`, id)
			d = read()
			if d.Resources["total"] != float64(0) || d.Resources["data_available"] != true {
				t.Fatal("confirmed empty inventory unavailable", d.Resources)
			}
		})
	}
}

type unavailableVideoUpdate struct{}

func (unavailableVideoUpdate) Request(context.Context, string) (updateState, error) {
	return updateState{}, errors.New("offline")
}

func TestProjectUpdateReportsRuntimeSeparateFromHistoricalReceipt(t *testing.T) {
	for _, controller := range []updateController{&fakeUpdateController{state: updateState{Status: "succeeded", InstalledRevision: "old-job-revision"}}, unavailableVideoUpdate{}} {
		s := Server{Updater: controller}
		r := httptest.NewRequest("GET", "/", nil)
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin}))
		w := httptest.NewRecorder()
		s.projectUpdateStatus(w, r)
		var got updateState
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || got.CurrentRevision != buildinfo.Current().Commit || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Code, got)
		}
		if got.Status == "succeeded" && got.InstalledRevision != "old-job-revision" {
			t.Fatal("historical receipt overwritten", got)
		}
	}
}

func TestServerHealthPreservesProviderBlockBesideRoutingFailure(t *testing.T) {
	db := adminTestDB(t)
	a, _, p := seedPanel(t, db, "http://fixture.invalid")
	sqlMust(t, db, `UPDATE accounts SET provider_state='TOKEN_INVALID' WHERE id=$1`, a)
	sqlMust(t, db, `INSERT INTO panel_routing_state(panel_id,state) VALUES($1,'FAILED')`, p)
	var raw []byte
	if err := db.QueryRow(dashboardCountsSQL).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Details []struct {
			ProviderState string `json:"provider_state"`
			RouteState    string `json:"route_state"`
		} `json:"server_health"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Details) != 1 || got.Details[0].ProviderState != "TOKEN_INVALID" || got.Details[0].RouteState != "FAILED" {
		t.Fatal(string(raw))
	}
}
