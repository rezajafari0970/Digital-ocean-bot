package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestResolvedFailurePruningPreservesUnfinishedEvidence(t *testing.T) {
	db, _, _ := recoveryFixture(t, "operation")
	// No ANALYZE, JIT override or deadline extension: exercise the cold plan.
	var raw []byte
	if err := db.QueryRow("EXPLAIN (FORMAT JSON) " + clearResolvedFailuresSQL).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var plans []struct {
		Plan struct {
			Cost float64 `json:"Total Cost"`
		}
		JIT json.RawMessage
	}
	if err := json.Unmarshal(raw, &plans); err != nil {
		t.Fatal(err)
	}
	t.Logf("cold pruning plan cost=%v jit=%s", plans[0].Plan.Cost, plans[0].JIT)
	// This small fixture must not select the expensive compilation path which
	// exhausted the unchanged ten-second recovery scan budget.
	if len(plans[0].JIT) != 0 {
		t.Fatal("small pruning fixture selected JIT compilation")
	}
	type candidate struct {
		kind, state, installer string
		selected, remove       bool
	}
	cases := []candidate{
		{"operation", "succeeded", "", false, true}, {"operation", "failed", "", false, true},
		{"operation", "running", "", false, false}, {"operation", "unknown", "", false, false},
		{"lifecycle", "DELETED", "", false, true}, {"lifecycle", "READY", "", false, false},
		{"lifecycle", "RETIRING", "", false, false},
		{"deployment", "READY", "", false, true}, {"deployment", "FAILED", "", false, true},
		{"deployment", "INSTALL_COMPLETE", "", false, true}, {"deployment", "INSTALL_FAILED", "", false, true},
		{"deployment", "INSTALL_ROLLED_BACK", "", false, true}, {"deployment", "PROVISIONING", "", false, false},
		{"deployment", "WAITING_INSTALLER", "", false, true},
		{"deployment", "WAITING_INSTALLER", "", true, false},
		{"deployment", "WAITING_INSTALLER", "legacy", false, false},
		{"missing", "", "", false, false},
	}
	for i, c := range cases {
		id := fmt.Sprintf("10000000-0000-0000-0000-%012d", i)
		sqlMust(t, db, fmt.Sprintf("INSERT INTO worker_item_failures(kind,item_id,failures,last_error,next_retry_at) VALUES('%s','%s',1,'original failure',now()+interval '1 hour')", c.kind, id))
		switch c.kind {
		case "operation":
			sqlMust(t, db, fmt.Sprintf("INSERT INTO operations VALUES('%s','%s','CREATE_DROPLET','%s',now())", id, recoveryAccount, c.state))
		case "lifecycle":
			sqlMust(t, db, fmt.Sprintf("INSERT INTO droplets VALUES('%s','%s')", id, c.state))
		case "deployment":
			profile := "{}"
			if c.installer != "" {
				profile = `{"installer_ref":{"name":"legacy"}}`
			}
			sqlMust(t, db, fmt.Sprintf("INSERT INTO deployments VALUES('%s','%s','%s','provision','%s',1,now())", id, recoveryAccount, c.state, profile))
			if c.selected {
				sqlMust(t, db, fmt.Sprintf("INSERT INTO deployment_installer_selections VALUES('%s',1)", id))
			}
		}
	}
	if err := (FailureStore{DB: db}).ClearResolvedChecked(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		var exists bool
		if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM worker_item_failures WHERE kind=$1 AND item_id=$2 AND last_error='original failure' AND next_retry_at>now())", c.kind, fmt.Sprintf("10000000-0000-0000-0000-%012d", i)).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists == c.remove {
			t.Fatalf("case %d %+v: retained=%t", i, c, exists)
		}
	}
}
