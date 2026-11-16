package configplane

import (
	"context"
	"testing"
	"time"
)

// Scheduling may open parallel discovery only after a real committed claim.
// A callback failure must still release that claim through the native defer.
func TestProfileDiscoveryReportsCommittedClaimAndReleasesOnPanic(t *testing.T) {
	db := cdnProofDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	calls := 0
	service := Service{DB: db, Claimed: func() { calls++ }}
	if err := service.RunDeleteOne(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("empty queue reported work")
	}
	_, profile, _ := cdnFixture(t, db)
	cdnExec(t, db, "UPDATE config_driver_profiles SET enabled=false WHERE id=$1", profile)
	cdnExec(t, db, "UPDATE config_driver_listeners SET next_attempt_at=now()-interval '1 second' WHERE profile_id=$1", profile)
	sentinel := "stop before remote execution"
	service.Claimed = func() {
		calls++
		var held bool
		if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM config_driver_listeners WHERE profile_id=$1 AND claim_token IS NOT NULL AND lease_until>now())", profile).Scan(&held); err != nil || !held {
			t.Fatalf("uncommitted claim: %v, %t", err, held)
		}
		panic(sentinel)
	}
	func() {
		defer func() {
			if r := recover(); r != sentinel {
				t.Fatalf("callback not reached or panic lost: %v", r)
			}
		}()
		if err := service.RunDeleteOne(ctx); err != nil {
			t.Fatal(err)
		}
	}()
	if calls != 1 {
		t.Fatalf("callbacks=%d", calls)
	}
	var held bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM config_driver_listeners WHERE profile_id=$1 AND (claim_token IS NOT NULL OR lease_until IS NOT NULL))", profile).Scan(&held); err != nil || held {
		t.Fatalf("claim cleanup failed: %v, %t", err, held)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if err := service.RunDeleteOne(cancelled); err == nil {
		t.Fatal("cancelled SQL discovery succeeded")
	}
	if calls != 1 {
		t.Fatal("cancelled discovery reported work")
	}
}
