package adminapi

import (
	"encoding/json"
	"testing"
)

func TestDashboardSeparatesProvisioningFromUnverified(t *testing.T) {
	db := adminTestDB(t)
	_, drop, _ := seedPanel(t, db, "http://fixture.invalid")
	check := func(unverified, provisioning int) {
		t.Helper()
		var raw []byte
		if err := db.QueryRow(dashboardCountsSQL).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var counts map[string]json.RawMessage
		if err := json.Unmarshal(raw, &counts); err != nil {
			t.Fatal(err)
		}
		for field, want := range map[string]int{"unverified_servers": unverified, "provisioning_servers": provisioning, "inactive_servers": 1} {
			var got int
			if err := json.Unmarshal(counts[field], &got); err != nil || got != want {
				t.Fatalf("%s got %d want %d: %v", field, got, want, err)
			}
		}
	}
	sqlMust(t, db, "UPDATE droplets SET state='PROVISIONING' WHERE id=$1", drop)
	check(0, 1)
	// No provider snapshot: the finished lifecycle remains genuinely unverified.
	sqlMust(t, db, "UPDATE droplets SET state='READY' WHERE id=$1", drop)
	check(1, 0)
}
