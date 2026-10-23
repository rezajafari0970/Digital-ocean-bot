package adminapi

import (
	"encoding/json"
	"testing"
)

// A history-sized fixture guards the account-list query's access path. A
// canonical-only index is insufficient: the newest legacy snapshot must win.
func TestLatestAccountSnapshotBoundsHistoryReads(t *testing.T) {
	db := adminTestDB(t)
	id := seedBuildAccount(t, db, "digitalocean")
	sqlMust(t, db, `INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical,created_at)
 SELECT gen_random_uuid(),$1,'digitalocean',2,
 jsonb_build_object('Limits',jsonb_build_object('DropletLimit',g),'padding',repeat(md5(g::text),128)),
 jsonb_build_object('Capacity',jsonb_build_object('ComputeLimit',g)),now()-g*interval '1 second'
 FROM generate_series(1,2500) g`, id)
	sqlMust(t, db, `INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical,created_at)
 VALUES(gen_random_uuid(),$1,'digitalocean',1,'{"Limits":{"DropletLimit":9999}}',NULL,now()+interval '1 second')`, id)
	sqlMust(t, db, `ANALYZE provider_snapshots`)
	const query = `SELECT COALESCE((canonical->'Capacity'->>'ComputeLimit')::int,(data->'Limits'->>'DropletLimit')::int) FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`
	var latest int
	if err := db.QueryRow(query, id).Scan(&latest); err != nil || latest != 9999 {
		t.Fatalf("latest legacy snapshot = %d, error=%v", latest, err)
	}
	var raw []byte
	if err := db.QueryRow(`EXPLAIN (ANALYZE, FORMAT JSON) `+query, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var plans []map[string]any
	if err := json.Unmarshal(raw, &plans); err != nil {
		t.Fatal(err)
	}
	visited := float64(0)
	found := false
	var walk func(map[string]any)
	walk = func(node map[string]any) {
		if node["Relation Name"] == "provider_snapshots" {
			found = true
			visited += node["Actual Rows"].(float64) * node["Actual Loops"].(float64)
		}
		if children, ok := node["Plans"].([]any); ok {
			for _, child := range children {
				walk(child.(map[string]any))
			}
		}
	}
	walk(plans[0]["Plan"].(map[string]any))
	if !found || visited > 1 {
		t.Fatalf("latest snapshot traversed %.0f history rows: %s", visited, raw)
	}
}
