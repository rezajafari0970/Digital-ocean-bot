package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http/httptest"
	"testing"
)

func TestAccountInventoryPendingCreationIsNotForeignOrManaged(t *testing.T) {
	db := adminTestDB(t)
	for _, kind := range []string{"pending", "regional", "failed_operation", "failed_deployment", "other_account", "wrong_key", "conflicting_resource", "retired"} {
		t.Run(kind, func(t *testing.T) {
			a := seedBuildAccount(t, db, "ovh")
			other := seedBuildAccount(t, db, "ovh")
			dep, profile := perfUUID(), perfUUID()
			owner := a
			key := "deploy:" + dep + ":create"
			opstate, depstate, resource := "verifying", "WAITING_RESOURCE", ""
			switch kind {
			case "regional":
				key += ":region:BHS5"
			case "failed_operation":
				opstate = "failed"
			case "failed_deployment":
				depstate = "FAILED"
			case "other_account":
				owner = other
			case "wrong_key":
				key = "deploy:" + perfUUID() + ":create"
			case "conflicting_resource":
				resource = "different"
			}
			sqlMust(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$2,'pending test','{}')", profile, a)
			sqlMust(t, db, "INSERT INTO deployments(id,account_id,profile_id,state,current_step,provider_id) VALUES($1,$2,$3,$4,'wait_resource',$5)", dep, a, profile, depstate, resource)
			sqlMust(t, db, "INSERT INTO operations(id,account_id,kind,idempotency_key,state,resource_id) VALUES(gen_random_uuid(),$1,'CREATE_DROPLET',$2,$3,'creating')", owner, key, opstate)
			if kind == "retired" {
				sqlMust(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES(gen_random_uuid(),$1,'creating','DELETED')", a)
			}
			obs := providers.Observation{Inventory: providers.Inventory{Servers: []providers.Server{{ID: "creating", State: providers.ServerStateProvisioning}, {ID: "unknown", State: providers.ServerStateProvisioning}}}}
			raw, _ := json.Marshal(obs)
			sqlMust(t, db, "INSERT INTO provider_snapshots(id,account_id,provider,data,canonical) VALUES(gen_random_uuid(),$1,'ovh','{}',$2)", a, raw)
			s := Server{DB: db}
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			r.SetPathValue("id", a)
			s.accountDashboard(w, r)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var got accountDashboard
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			want := "foreign"
			if kind == "pending" || kind == "regional" {
				want = "creation_pending"
			}
			for _, x := range got.Droplets {
				expected := want
				if x["id"] == "unknown" {
					expected = "foreign"
				}
				if x["ownership"] != expected || x["managed"] != false {
					t.Fatal("incorrect ownership", kind, x)
				}
			}
			if got.Resources["managed"] != float64(0) {
				t.Fatal("pending proof granted management", got.Resources)
			}
		})
	}
}
