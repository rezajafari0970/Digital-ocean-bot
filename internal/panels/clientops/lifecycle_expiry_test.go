package clientops

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// Queue downtime must not POST expired credentials or abort clients which
// really committed before a lost response. Identity conflicts stay fail-closed.
func TestLifecycleExpiredPlanFreshRecovery(t *testing.T) {
	for _, scenario := range []string{"absent", "partial_committed", "identity_conflict"} {
		t.Run(scenario, func(t *testing.T) {
			db, j, rt, state := lifecycleFixture(t)
			ctx := context.Background()
			if _, err := db.Exec("UPDATE bulk_lifecycle_scopes SET lifetime_seconds=1"); err != nil {
				t.Fatal(err)
			}
			if !planLife(t, j, rt) {
				t.Fatal("plan")
			}
			job, ok, err := j.Claim(ctx)
			if err != nil || !ok {
				t.Fatal(ok, err)
			}
			var p BulkPayload
			if err = json.Unmarshal(job.Payload, &p); err != nil {
				t.Fatal(err)
			}
			if scenario != "absent" {
				c := p.Clients[0]
				if scenario == "identity_conflict" {
					c.ID = "other-identity"
				}
				state.clients[c.Email] = c
			}
			err = (Executor{Journal: j}).executeRuntime(ctx, rt, job)
			if scenario == "identity_conflict" {
				if !errors.Is(err, ErrClientConflict) || errors.Is(err, ErrLifecycleExpired) {
					t.Fatal(err)
				}
				var planned int
				db.QueryRow("SELECT count(*) FROM bulk_user_ownership WHERE state='PLANNED'").Scan(&planned)
				if planned != len(p.Clients) {
					t.Fatal("conflict ownership changed", planned)
				}
				if state.postCreates != 0 {
					t.Fatal("POST on conflict")
				}
				return
			}
			if !errors.Is(err, ErrLifecycleExpired) {
				t.Fatal(err)
			}
			if state.postCreates != 0 || state.postUpdates != 0 || state.postDeletes != 0 {
				t.Fatal("expired plan mutated panel")
			}
			if err = j.retireLifecycle(ctx, job, "planned lifetime elapsed before POST", "LIFETIME_ELAPSED_BEFORE_POST"); err != nil {
				t.Fatal(err)
			}
			got, err := j.Get(ctx, job.ID)
			if err != nil || got.State != StateObsolete {
				t.Fatal(got.State, err)
			}
			var active, aborted int
			db.QueryRow("SELECT count(*) FILTER(WHERE state='ACTIVE'),count(*) FILTER(WHERE state='ABORTED') FROM bulk_user_ownership").Scan(&active, &aborted)
			wantActive := 0
			if scenario == "partial_committed" {
				wantActive = 1
			}
			if active != wantActive || aborted != len(p.Clients)-wantActive {
				t.Fatal(active, aborted)
			}
			if err = j.retireLifecycle(ctx, job, "again", "AGAIN"); err == nil {
				t.Fatal("stale completion accepted")
			}
			var enabled, killed bool
			db.QueryRow("SELECT enabled,kill_switch FROM client_mutation_execution_gate").Scan(&enabled, &killed)
			if !enabled || killed {
				t.Fatal("safe retirement closed gate")
			}
			if state.clients["manual"].ID != "manual-id" {
				t.Fatal("manual changed")
			}
			if !planLife(t, j, rt) {
				t.Fatal("retired plan still blocks lifecycle")
			}
		})
	}
}
