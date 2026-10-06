package app

import (
	"context"
	"testing"
)

func TestSSHKeyBootQuarantineScopeAndRecovery(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	f := newBootstrapFixture(t, db)
	execBootstrap(t, db, `UPDATE deployments SET state='FAILED',last_error='INITIAL_SSH_BUDGET_EXHAUSTED: publickey',profile_snapshot='{"image":"26","region":"fra","size":"1gb"}' WHERE id=$1`, f.d.ID)
	get := func(region string) bool {
		t.Helper()
		q, err := bootImageQuarantine(ctx, db, f.d.AccountID, region, "1gb")
		if err != nil {
			t.Fatal(err)
		}
		return q["26"]
	}
	if get("fra") {
		t.Fatal("single failure quarantined")
	}
	execBootstrap(t, db, `INSERT INTO deployments(id,account_id,profile_id,state,current_step,last_error,profile_snapshot) SELECT gen_random_uuid(),account_id,profile_id,state,current_step,last_error,profile_snapshot FROM deployments WHERE id=$1`, f.d.ID)
	if !get("fra") || get("ams") {
		t.Fatal("quarantine scope incorrect")
	}
	execBootstrap(t, db, `UPDATE deployments SET state='PANEL_COMPLETE',updated_at=now()+interval '1 second' WHERE id=$1`, f.d.ID)
	if get("fra") {
		t.Fatal("new successful boot did not clear failures")
	}
	execBootstrap(t, db, `UPDATE deployments SET state='FAILED',updated_at=now()-interval '2 hours' WHERE account_id=$1`, f.d.AccountID)
	if get("fra") {
		t.Fatal("expired failures retained")
	}
	execBootstrap(t, db, `INSERT INTO deployments(id,account_id,profile_id,state,current_step,profile_snapshot) SELECT gen_random_uuid(),account_id,profile_id,'PLANNED','create',profile_snapshot FROM deployments WHERE id=$1`, f.d.ID)
	if !get("fra") {
		t.Fatal("second half-open boot probe permitted")
	}
	bootstrapComplete(t, db, f, "ssh")
	if get("fra") {
		t.Fatal("successful SSH did not clear boot-only quarantine")
	}
	execBootstrap(t, db, `UPDATE deployments SET profile_snapshot=profile_snapshot-'image' WHERE account_id=$1`, f.d.AccountID)
	if get("fra") {
		t.Fatal("missing image poisoned selection")
	}
}
