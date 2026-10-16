package adminapi

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"os"
	"path/filepath"
	"testing"
)

func TestFleetCoveragePopulatedRetainedUpgrade(t *testing.T) {
	db := adminTestDB(t)
	raw, err := os.ReadFile("../../migrations/000182_fleet_client_coverage.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Isolated fixture already has the additive schema. Replay the immutable
	// upgrade against an actual populated, globally enabled pre-upgrade policy.
	sqlMust(t, db, "DELETE FROM schema_migrations WHERE version='000182_fleet_client_coverage'")
	sqlMust(t, db, "UPDATE bulk_lifecycle_control SET enabled=true,auto_enroll=true,max_active_scopes=64,operation_budget=123,max_batch_size=11; UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,concurrency=1,panel_id=NULL,inbound_id=NULL")
	dir := t.TempDir()
	path := filepath.Join(dir, "000182_fleet_client_coverage.up.sql")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	runner := migrate.Runner{DB: db, Dir: dir, PreserveData: true}
	if err = runner.Up(context.Background()); err != nil {
		t.Fatal("populated retained upgrade failed", err)
	}
	var limit, budget, chunk int
	if err = db.QueryRow("SELECT max_active_scopes,operation_budget,max_batch_size FROM bulk_lifecycle_control").Scan(&limit, &budget, &chunk); err != nil || limit != 0 || budget != 123 || chunk != 11 {
		t.Fatal(limit, budget, chunk, err)
	}
	// The exact-version allowance must not permit unrelated policy changes.
	sqlMust(t, db, "DELETE FROM schema_migrations WHERE version='000182_fleet_client_coverage'; UPDATE bulk_lifecycle_control SET max_active_scopes=64")
	if err = os.WriteFile(path, append(raw, []byte("\nUPDATE bulk_lifecycle_control SET operation_budget=999;\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if err = runner.Up(context.Background()); err == nil {
		t.Fatal("unrelated policy overwrite accepted")
	}
	if err = db.QueryRow("SELECT max_active_scopes,operation_budget FROM bulk_lifecycle_control").Scan(&limit, &budget); err != nil || limit != 64 || budget != 123 {
		t.Fatal("failed upgrade did not roll back", limit, budget, err)
	}
}
