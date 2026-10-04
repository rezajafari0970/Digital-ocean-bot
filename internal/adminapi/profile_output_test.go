package adminapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProfileOutputExcludesManualAndOtherOwnershipClass(t *testing.T) {
	db := adminTestDB(t)
	_, _, panel := seedPanel(t, db, "http://panel.test")
	sqlMust(t, db, `INSERT INTO reality_config_profiles(route_class,ports,target_users_per_inbound) VALUES('DIRECT','[443]',1)`)
	sqlMust(t, db, `UPDATE residential_routing_control SET enabled=true,fleet=true`)
	var rev int64
	if err := db.QueryRow(`SELECT revision FROM residential_routing_control`).Scan(&rev); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, `INSERT INTO panel_routing_state(panel_id,revision,state,verified_at) VALUES($1,$2,'APPLIED',now())`, panel, rev)
	var gen string
	if err := db.QueryRow(`INSERT INTO bulk_user_generations(panel_id,inbound_id,purpose,marker) VALUES($1,1,'POLICY','output-proof') RETURNING id::text`, panel).Scan(&gen); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"owned", "manual", "wrong-class", "deleted"} {
		sqlMust(t, db, `INSERT INTO panel_client_routes(panel_id,client_id,email,route_class,effective_class,revision) VALUES($1,$2,$2,'DIRECT','DIRECT',$3)`, panel, id, rev)
		sqlMust(t, db, `INSERT INTO output_config_snapshots(panel_id,uri,client_id) VALUES($1,$2,$3)`, panel, "vless://"+id+"@panel.test", id)
		if id == "manual" {
			continue
		}
		cls, state := "DIRECT", "ACTIVE"
		if id == "wrong-class" {
			cls = "RESIDENTIAL"
		}
		if id == "deleted" {
			state = "DELETED"
		}
		sqlMust(t, db, `INSERT INTO bulk_user_ownership(generation_id,client_id,email,route_class,state) VALUES($1,$2,$2,$3,$4)`, gen, id, cls, state)
	}
	w := httptest.NewRecorder()
	(&Server{DB: db}).outputSnapshotClass(w, httptest.NewRequest("GET", "/", nil), "DIRECT")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "vless://owned@panel.test" {
		t.Fatal(w.Code, w.Body.String())
	}
}
