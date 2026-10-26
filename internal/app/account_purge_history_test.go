package app

import (
	"context"
	"errors"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"testing"
)

func TestAccountHistoryPruningRechecksConfirmationAndPreservesLiveState(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	other := newBootstrapFixture(t, db)
	ctx := context.Background()
	execBootstrap(t, db, "UPDATE accounts SET enabled=false,provider_state='LOCKED',deletion_requested_at=now()-interval '3 minutes',runtime_status='DELETE_PENDING' WHERE id=$1", f.d.AccountID)
	execBootstrap(t, db, "INSERT INTO provider_snapshots(id,account_id,provider,data,canonical,created_at) SELECT gen_random_uuid(),$1,'vultr','{}','{}',now() FROM generate_series(1,1100)", f.d.AccountID)
	execBootstrap(t, db, "INSERT INTO provider_snapshots(id,account_id,provider,data,canonical,created_at) VALUES(gen_random_uuid(),$1,'vultr','{}','{}',now())", other.d.AccountID)
	confirmation, e := ReadAccountPurgeConfirmation(ctx, db, f.d.AccountID)
	if e != nil {
		t.Fatal(e)
	}
	execBootstrap(t, db, "INSERT INTO worker_recovery_checkpoints(kind,item_id,account_id) VALUES('deployment',$1,$2)", f.d.ID, f.d.AccountID)
	if _, _, e = pruneSavedAccountHistory(ctx, db, f.d.AccountID, "LOCKED", 1, confirmation); !errors.Is(e, worker.ErrRecoveryCheckpointPending) {
		t.Fatal("pruned during unresolved recovery", e)
	}
	execBootstrap(t, db, "DELETE FROM worker_recovery_checkpoints WHERE account_id=$1", f.d.AccountID)
	done, n, e := pruneSavedAccountHistory(ctx, db, f.d.AccountID, "LOCKED", 1, confirmation)
	if e != nil || !done || n < 1100 {
		t.Fatal("history pruning", done, n, e)
	}
	for _, q := range []string{"SELECT count(*) FROM accounts WHERE id=$1", "SELECT count(*) FROM droplets WHERE account_id=$1"} {
		var n int
		if e = db.QueryRow(q, f.d.AccountID).Scan(&n); e != nil || n != 1 {
			t.Fatal("functional state removed before final commit", n, e)
		}
	}
	var kept int
	if e = db.QueryRow("SELECT count(*) FROM provider_snapshots WHERE account_id=$1", other.d.AccountID).Scan(&kept); e != nil || kept != 1 {
		t.Fatal("unrelated history removed", e)
	}
	execBootstrap(t, db, "UPDATE accounts SET enabled=true WHERE id=$1", f.d.AccountID)
	if _, _, e = pruneSavedAccountHistory(ctx, db, f.d.AccountID, "LOCKED", 1, confirmation); !errors.Is(e, ErrAccountPurgeConflict) {
		t.Fatal("stale confirmation accepted", e)
	}
}

func TestAccountPurgeRetryUsesSafeCodes(t *testing.T) {
	for _, code := range []string{"57014", "55P03", "40P01", "40001", "42501", "23503"} {
		state, msg, delay := accountPurgeRetry(&pq.Error{Code: pq.ErrorCode(code), Message: "secret fixture"})
		if state != "RETRY" || delay < 5 || delay > 60 || msg == "secret fixture" {
			t.Fatal("unsafe retry", code, state, msg, delay)
		}
	}
}
