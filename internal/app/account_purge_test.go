package app

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"testing"
)

func TestAccountPurgeWaitsForDurableRecoveryReconciliation(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	f := newBootstrapFixture(t, db)
	execBootstrap(t, db, "UPDATE accounts SET enabled=false,provider_state='LOCKED',deletion_requested_at=now()-interval '3 minutes',runtime_status='DELETE_PENDING' WHERE id=$1", f.d.AccountID)
	execBootstrap(t, db, "INSERT INTO worker_recovery_checkpoints(kind,item_id,account_id) VALUES('deployment',$1,$2)", f.d.ID, f.d.AccountID)
	if err := PurgeSavedAccount(ctx, db, f.d.AccountID, "LOCKED", 1); !errors.Is(err, ErrAccountPurgeConflict) {
		t.Fatal("purge erased unresolved native evidence", err)
	}
	w := worker.Worker{Store: worker.RecoveryStore{DB: db}, Failures: worker.FailureStore{DB: db}, Handler: RecoveryHandler{Container: Container{DB: db}}}
	if err := w.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if err := PurgeSavedAccount(ctx, db, f.d.AccountID, "LOCKED", 1); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM worker_recovery_checkpoints WHERE account_id=$1", f.d.AccountID).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", f.d.AccountID).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
}
