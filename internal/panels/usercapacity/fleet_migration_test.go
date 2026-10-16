package usercapacity

import (
	"os"
	"testing"
)

func TestFleetCoverageMigrationPreservesExplicitAndPausedLimits(t *testing.T) {
	db := admissionTestDB(t)
	raw, err := os.ReadFile("../../../migrations/000182_fleet_client_coverage.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, setup string
		want        int
	}{
		{"global default", "", 0},
		{"custom limit", "UPDATE bulk_lifecycle_control SET max_active_scopes=17", 17},
		{"custom maximum", "UPDATE bulk_lifecycle_control SET max_active_scopes=256", 256},
		{"paused lifecycle", "UPDATE bulk_lifecycle_control SET enabled=false", 64},
		{"paused execution", "UPDATE client_mutation_execution_gate SET enabled=false", 64},
		{"kill switch", "UPDATE client_mutation_execution_gate SET kill_switch=true", 64},
		{"scoped execution", "UPDATE client_mutation_execution_gate SET panel_id='55555555-5555-4555-8555-555555555555'", 64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.Exec("UPDATE bulk_lifecycle_control SET enabled=true,auto_enroll=true,max_active_scopes=64; UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,panel_id=NULL,inbound_id=NULL,concurrency=1;" + tc.setup); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(string(raw)); err != nil {
				t.Fatal(err)
			}
			var n int
			if err := db.QueryRow("SELECT max_active_scopes FROM bulk_lifecycle_control").Scan(&n); err != nil || n != tc.want {
				t.Fatal(n, tc.want, err)
			}
		})
	}
}
