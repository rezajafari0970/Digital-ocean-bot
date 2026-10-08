package app

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"os"
	"path/filepath"
	"strings"
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

func TestAccountBrowserArtifactsScopedAndSymlinkSafe(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	id := "43910bab-130c-408c-8e16-f94613e87f50"
	other := "58f24b02-22e9-4e0b-ac48-b2d5d3c2f7e"
	write := func(p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(outside, "keep"))
	write(filepath.Join(root, other, "keep"))
	write(filepath.Join(root, id, "Cache", "cookie"))
	if err := os.Symlink(outside, filepath.Join(root, id, "outside")); err != nil {
		t.Fatal(err)
	}
	if err := removeAccountBrowserSession(root, id); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(outside, "keep"), filepath.Join(root, other, "keep")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal("unrelated data removed", err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, id)); err != nil {
		t.Fatal(err)
	}
	if err := removeAccountBrowserSession(root, id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep")); err != nil {
		t.Fatal("target symlink followed", err)
	}
	for _, bad := range []string{"", "..", "../" + id, strings.ToUpper(id), strings.ReplaceAll(id, "-", ""), "g" + id[1:]} {
		if err := removeAccountBrowserSession(root, bad); err == nil {
			t.Fatal("invalid ID accepted", bad)
		}
	}
	linked := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(root, linked); err != nil {
		t.Fatal(err)
	}
	if err := removeAccountBrowserSession(linked, id); err == nil {
		t.Fatal("symlink root accepted")
	}
	for i := 0; i < 2; i++ {
		if err := removeAccountBrowserSession(root, id); err != nil {
			t.Fatal("non-idempotent removal", err)
		}
	}
	if err := removeAccountBrowserSession(filepath.Join(t.TempDir(), "absent"), id); err != nil {
		t.Fatal(err)
	}
	fileRoot := filepath.Join(t.TempDir(), "file")
	write(fileRoot)
	if err := removeAccountBrowserSession(fileRoot, id); err == nil {
		t.Fatal("invalid root accepted")
	}
}

func TestAccountPurgeArtifactsFailureRollbackAndCommitBoundary(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	ctx := context.Background()
	root := t.TempDir()
	id := f.d.AccountID
	execBootstrap(t, db, "UPDATE accounts SET enabled=false,provider_state='LOCKED',deletion_requested_at=now()-interval '3 minutes',runtime_status='DELETE_PENDING' WHERE id=$1", id)
	makeProfile := func() {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, id), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, id, "cookie"), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	makeProfile()
	purge := func(remove func(string) error) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err = lockAccountPurge(ctx, tx, id); err != nil {
			return err
		}
		return commitAccountPurgeWithArtifacts(ctx, tx, id, remove)
	}
	failure := errors.New("fixture filesystem failure")
	if err := purge(func(got string) error {
		if got != id {
			t.Error("not canonical", got)
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	exists := func(want int) {
		t.Helper()
		var n int
		if err := db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", id).Scan(&n); err != nil || n != want {
			t.Fatal("DB purge state", n, err)
		}
	}
	exists(1)
	if _, err := os.Stat(filepath.Join(root, id, "cookie")); err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, "CREATE FUNCTION reject_purge_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture commit fault'; END $$")
	execBootstrap(t, db, "CREATE CONSTRAINT TRIGGER purge_commit_fault AFTER DELETE ON accounts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_purge_commit()")
	remove := func(got string) error { return removeAccountBrowserSession(root, got) }
	if err := purge(remove); err == nil {
		t.Fatal("commit fault hidden")
	}
	exists(1)
	if _, err := os.Stat(filepath.Join(root, id)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("documented filesystem/DB boundary differs", err)
	}
	execBootstrap(t, db, "DROP TRIGGER purge_commit_fault ON accounts")
	makeProfile()
	if err := purge(remove); err != nil {
		t.Fatal(err)
	}
	exists(0)
	if _, err := os.Stat(filepath.Join(root, id)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("profile remains", err)
	}
}

func TestPurgeRejectsNoncanonicalIDBeforeAnyFence(t *testing.T) {
	id := "43910BAB-130C-408C-8E16-F94613E87F50"
	ctx := context.Background()
	// Nil dependencies prove rejection precedes both DB access and native fencing.
	if err := PurgeSavedAccount(ctx, nil, id, "LOCKED", 0); !errors.Is(err, ErrAccountPurgeConflict) {
		t.Fatal(err)
	}
	if err := lockAccountPurge(ctx, nil, id); !errors.Is(err, ErrAccountPurgeConflict) {
		t.Fatal(err)
	}
	if err := commitAccountPurgeWithArtifacts(ctx, nil, id, nil); !errors.Is(err, ErrAccountPurgeConflict) {
		t.Fatal(err)
	}
}
func TestAccountBrowserRootReplacementCannotRedirectRemoval(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "sessions")
	outside := t.TempDir()
	id := "43910bab-130c-408c-8e16-f94613e87f50"
	for _, base := range []string{root, outside} {
		if err := os.MkdirAll(filepath.Join(base, id), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, id, "keep"), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	anchored, err := openAccountBrowserRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer anchored.Close()
	moved := filepath.Join(parent, "original")
	if err = os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	if err = anchored.RemoveAll(id); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(outside, id, "keep")); err != nil {
		t.Fatal("root replacement redirected removal", err)
	}
	if _, err = os.Stat(filepath.Join(moved, id)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("anchored removal missing", err)
	}
	if err = removeAccountBrowserSession(root, id); err == nil {
		t.Fatal("new call accepted replaced root")
	}
}
