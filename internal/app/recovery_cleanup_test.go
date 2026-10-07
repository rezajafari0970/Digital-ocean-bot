package app

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"strings"
	"testing"
	"time"
)

func TestRecoveryCleanupBoundedAndPreservesOriginalError(t *testing.T) {
	db := fenceTestDB(t)
	_, err := db.Exec(`ALTER TABLE accounts ADD COLUMN provider_state text;
 CREATE TABLE operations(id text,account_id text,resource_id text,created_at timestamptz,lock_version bigint,state text,error_code text,error_message text,updated_at timestamptz);
 INSERT INTO accounts(id,provider_state) VALUES('a','ACTIVE');
 INSERT INTO operations VALUES('o','a','',now(),1,'running','','',now());
 CREATE FUNCTION reject_original() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'original recovery fixture failure'; END$$;
 CREATE TRIGGER reject_original BEFORE UPDATE ON operations FOR EACH ROW EXECUTE FUNCTION reject_original();`)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("LOCK TABLE accounts IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	done := make(chan error, 1)
	go func() {
		done <- (RecoveryHandler{Container: Container{DB: db}}).RecoverOperation(ctx, worker.RecoveryItem{ID: "o", AccountID: "a", Kind: "DELETE_DROPLET"})
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "original recovery fixture failure") {
			t.Fatal("original error lost", err)
		}
		if time.Since(started) > 7*time.Second {
			t.Fatal("cleanup not bounded")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("deferred bookkeeping ignored cleanup deadline")
	}
}
