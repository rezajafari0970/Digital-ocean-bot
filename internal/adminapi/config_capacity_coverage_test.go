package adminapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestCapacityCoverageRetainsStaleAndMissingInbounds(t *testing.T) {
	db := adminTestDB(t)
	account, _, panel := seedPanel(t, db, "http://coverage.test")
	for _, id := range []int{1, 2, 3, 4} {
		sqlMust(t, db, `INSERT INTO panel_inbound_inventory(panel_id,remote_id,port,protocol,transport,security,enabled,present,raw_hash,first_seen_at,last_seen_at) VALUES($1,$2,443,'vless','tcp','reality',true,true,'fixture',now(),now())`, panel, id)
	}
	sqlMust(t, db, `INSERT INTO user_capacity_snapshots(panel_id,inbound_id,port,target_users,active_users,deficit,observed_at) VALUES
 ($1,1,443,4,3,1,now()),($1,2,443,6,5,1,now()-interval '2 minutes'),($1,4,443,999,999,0,now()+interval '1 hour')`, panel)
	read := func() map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		(&Server{DB: db}).configCapacity(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := read()
	coverage := out["coverage"].(map[string]any)
	if coverage["eligible"] != float64(4) || coverage["observed"] != float64(2) || coverage["fresh"] != float64(1) || coverage["stale"] != float64(1) || coverage["missing"] != float64(2) || out["active"] != float64(8) || out["target"] != float64(10) {
		t.Fatal(out)
	}
	for _, row := range out["inbounds"].([]any) {
		r := row.(map[string]any)
		if r["observation_status"] == "missing" && r["active"] != nil {
			t.Fatal("unknown presented as zero", r)
		}
	}
	sqlMust(t, db, "UPDATE user_capacity_snapshots SET observed_at=now()-interval '31 seconds' WHERE panel_id=$1 AND inbound_id=1", panel)
	next := read()
	if next["active"] != out["active"] || len(next["inbounds"].([]any)) != 4 {
		t.Fatal("aging erased capacity", next)
	}
	sqlMust(t, db, "UPDATE accounts SET enabled=false WHERE id=$1", account)
	if len(read()["inbounds"].([]any)) != 0 {
		t.Fatal("disabled account included")
	}
}
