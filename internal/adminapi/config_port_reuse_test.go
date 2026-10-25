package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configplane"
)

func TestProfilePortReuseIsolatesPendingDeletion(t *testing.T) {
	for _, mode := range []string{"CONFIRMED", "PREPARED", "SENT", "automatic"} {
		t.Run(mode, func(t *testing.T) {
			db := adminTestDB(t)
			account, _, panel := seedPanel(t, db, "http://127.0.0.1:19001")
			_, _, otherPanel := seedPanel(t, db, "http://127.0.0.1:19002")
			sqlMust(t, db, "UPDATE config_control_authority SET unified=true WHERE singleton")
			api := &Server{DB: db}
			port := 443
			if mode == "automatic" {
				port = 10000
			}
			request := configplane.Profile{Name: "Port owner", Enabled: true, Fleet: true, Classes: []string{"DIRECT"}, Count: 1, Spec: configdriver.Spec{Protocol: "vmess", Network: "tcp", Security: "none", Port: port}}
			code, old := profileSave(t, api, request, "POST")
			if code != 200 {
				t.Fatal(code)
			}
			raw, _ := json.Marshal(old.Spec)
			mutation := mode
			var remote any
			if mode == "CONFIRMED" || mode == "automatic" {
				remote = int64(123)
				mutation = "CONFIRMED"
			}
			key := "dob:profile:" + old.ID
			sqlMust(t, db, "INSERT INTO config_driver_listeners(profile_id,panel_id,managed_key,credential_ref,applied_spec,remote_id,mutation_state,mutation_spec,state) VALUES($1,$2,$3,'fixture',$4,$5,$6,$4,'BLOCKED')", old.ID, panel, key, raw, remote, mutation)
			if remote != nil {
				sqlMust(t, db, "INSERT INTO panel_inbound_inventory(panel_id,remote_id,remark,port,raw_hash,first_seen_at,last_seen_at) VALUES($1,123,$2,$3,'fixture',now(),now())", panel, key, port)
			}
			// Pausing does not surrender native ownership. Only committed deletion can
			// detach the fleet-wide reservation, and legacy authority keeps its guard.
			old.Enabled = false
			code, old = profileSave(t, api, old, "PUT")
			if code != 200 {
				t.Fatal(code)
			}
			if code, _ := profileSave(t, api, request, "POST"); code != 409 {
				t.Fatal("paused owner lost its reservation", code)
			}
			sqlMust(t, db, "UPDATE accounts SET provider_state='TOKEN_INVALID' WHERE id=$1", account)
			body, _ := json.Marshal(map[string]any{"revision": old.Revision})
			r := httptest.NewRequest("DELETE", "/", bytes.NewReader(body))
			r.SetPathValue("id", old.ID)
			receipt := httptest.NewRecorder()
			api.deleteConfigProfile(receipt, r)
			if receipt.Code != 202 {
				t.Fatal(receipt.Code)
			}
			sqlMust(t, db, "UPDATE config_control_authority SET unified=false WHERE singleton")
			if code, _ := profileSave(t, api, request, "POST"); code != 409 {
				t.Fatal("legacy reservation weakened", code)
			}
			sqlMust(t, db, "UPDATE config_control_authority SET unified=true WHERE singleton")
			if remote != nil {
				sqlMust(t, db, "UPDATE panel_inbound_inventory SET remark='unowned' WHERE panel_id=$1", panel)
				if code, _ := profileSave(t, api, request, "POST"); code != 409 {
					t.Fatal("unowned inventory excluded", code)
				}
				sqlMust(t, db, "UPDATE panel_inbound_inventory SET remark=$2 WHERE panel_id=$1", panel, key)
			}
			sqlMust(t, db, "INSERT INTO panel_inbound_inventory(panel_id,remote_id,remark,port,raw_hash,first_seen_at,last_seen_at,enabled) VALUES($1,999,'manual-disabled',$2,'fixture',now(),now(),false)", otherPanel, port)
			if code, _ := profileSave(t, api, request, "POST"); code != 409 {
				t.Fatal("disabled manual inbound ignored", code)
			}
			sqlMust(t, db, "UPDATE panel_inbound_inventory SET present=false WHERE panel_id=$1", otherPanel)
			if mode == "automatic" {
				request.Spec.Port = 0
			}
			code, replacement := profileSave(t, api, request, "POST")
			if code != 200 || replacement.Spec.Port != port {
				t.Fatal("single pending panel blocked replacement", code, replacement.Spec.Port)
			}
			duplicate := request
			duplicate.Spec.Port = port
			if code, _ := profileSave(t, api, duplicate, "POST"); code != 409 {
				t.Fatal("active desired collision accepted", code)
			}
			if _, err := db.Exec("INSERT INTO config_driver_profiles(id,name,spec,enabled,fleet,classes) VALUES('00000000-0000-4000-8000-000000000099','collision',$1,true,true,ARRAY['DIRECT'])", raw); err == nil {
				t.Fatal("database active reservation guard bypassed")
			}
			// Access recovery must not make the replacement overwrite the old socket.
			sqlMust(t, db, "UPDATE accounts SET provider_state='ACTIVE' WHERE id=$1", account)
			if err := (configplane.Service{DB: db}).Reconcile(context.Background(), replacement.ID, panel); err == nil || err.Error() != "profile_port_cleanup_pending" {
				t.Fatal("affected panel was not fenced before credentials/native access", err)
			}
			var kept bool
			if err := db.QueryRow("SELECT l.state='BLOCKED' AND l.mutation_state=$3 AND p.deleted_at IS NULL AND p.delete_requested_at IS NOT NULL FROM config_driver_listeners l JOIN config_driver_profiles p ON p.id=l.profile_id WHERE l.profile_id=$1 AND l.panel_id=$2", old.ID, panel, mutation).Scan(&kept); err != nil || !kept {
				t.Fatal("old cleanup ownership lost", kept, err)
			}
			down, err := os.ReadFile("../../migrations/000198_profile_port_cleanup_scope.down.sql")
			if err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(string(down))
			_ = tx.Rollback()
			if err == nil {
				t.Fatal("rollback accepted incompatible pending replacements")
			}
			replacement.Name = "Replacement retained"
			if code, _ := profileSave(t, api, replacement, "PUT"); code != 200 {
				t.Fatal("failed rollback changed reservation policy", code)
			}
		})
	}
}
