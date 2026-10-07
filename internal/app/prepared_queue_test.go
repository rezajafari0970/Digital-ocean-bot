package app

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/observability"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"testing"
)

func TestPreparedSnapshotAtomicVisibilityAndRecoveryFence(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	d, _, err := (workflow.SQLStore{DB: tx}).Reserve(ctx, workflow.Request{AccountID: f.d.AccountID, ProfileID: f.d.ProfileID})
	if err != nil {
		t.Fatal(err)
	}
	if err = (workflow.ProfileStore{DB: tx}).AttachSnapshot(ctx, d.ID, workflow.ProfileSnapshot{Image: "selected-image", Region: "selected-region"}); err != nil {
		t.Fatal(err)
	}
	if _, err = (workflow.SQLStore{DB: db}).Get(ctx, d.ID, d.AccountID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("incomplete reservation exposed: %v", err)
	}
	release, err := (workflow.PostgresRunLease{DB: db}).Acquire(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = (RecoveryHandler{Container: Container{DB: db}}).RecoverDeployment(ctx, worker.RecoveryItem{ID: d.ID, AccountID: d.AccountID}); err != nil {
		t.Fatalf("recovery bypassed preparation lease: %v", err)
	}
	snap, err := (workflow.ProfileStore{DB: db}).SnapshotForDeployment(ctx, d.ID)
	if err != nil || snap.Image != "selected-image" {
		t.Fatal(snap, err)
	}
}
func TestPreparedReadinessNeedsWorkerProgress(t *testing.T) {
	db := installerBootstrapDB(t)
	h := observability.Health{DB: db, RequireWorker: true}
	ctx := context.Background()
	if h.Readiness(ctx).Status == "ready" {
		t.Fatal("missing worker accepted")
	}
	execBootstrap(t, db, `INSERT INTO worker_heartbeats(worker_id,kind,last_seen_at,metadata) VALUES('fixture','production',now(),jsonb_build_object('recovery_scan_unix',extract(epoch FROM now())::bigint,'scheduler_scan_unix',extract(epoch FROM now())::bigint,'lifecycle_scan_unix',extract(epoch FROM now())::bigint,'lifecycle_lanes',jsonb_build_object('in_flight',0,'stalled',0),'client_mutation',jsonb_build_object('state','IDLE','finished_unix',extract(epoch FROM now())::bigint)))`)
	if h.Readiness(ctx).Status != "ready" {
		t.Fatal("fresh worker rejected")
	}
	execBootstrap(t, db, `UPDATE worker_heartbeats SET metadata=jsonb_set(metadata,'{recovery_scan_unix}','0'::jsonb)`)
	if h.Readiness(ctx).Status == "ready" {
		t.Fatal("heartbeat without progress accepted")
	}
}
