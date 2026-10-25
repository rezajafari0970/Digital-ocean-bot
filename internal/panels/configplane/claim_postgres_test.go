package configplane

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

// Reproduce a briefly busy profile row while independent destination claims
// arrive. All callers must receive distinct work before the shared lock is
// released, not block behind an editor or return an
// empty queue merely because another claimant updates the fairness timestamp.
func TestProfileClaimsDoNotDiscardIndependentDestinations(t *testing.T) {
	db := cdnProofDB(t)
	db.SetMaxOpenConns(16)
	_, profile, _ := cdnFixture(t, db)
	for i := 1; i < 8; i++ {
		_, other, _ := cdnFixture(t, db)
		cdnExec(t, db, "UPDATE config_driver_listeners SET profile_id=$1 WHERE profile_id=$2", profile, other)
	}
	cdnExec(t, db, "UPDATE config_driver_listeners SET next_attempt_at=now()-interval '1 second'")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	held, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback()
	if _, err = held.ExecContext(ctx, "SELECT id FROM config_driver_profiles WHERE id=$1 FOR UPDATE", profile); err != nil {
		t.Fatal(err)
	}
	type result struct {
		id, panel string
		err       error
	}
	results := make(chan result, 8)
	service := Service{DB: db}
	for i := 0; i < 8; i++ {
		go func() {
			claim, e := sanaei.UUIDv4()
			if e != nil {
				results <- result{err: e}
				return
			}
			id, panel, e := service.claimProfileWork(ctx, claim, "TRUE", "l.panel_id")
			results <- result{id, panel, e}
		}()
	}
	seen := map[string]bool{}
	for i := 0; i < 8; i++ {
		select {
		case r := <-results:
			if r.err != nil || r.id != profile || seen[r.panel] || r.panel == "" {
				t.Errorf("lost or duplicate independent claim: %#v", r)
			}
			seen[r.panel] = true
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if err = held.Commit(); err != nil { t.Fatal(err) }
	if len(seen) != 8 {
		t.Fatalf("claimed %d unique panels; want 8", len(seen))
	}
	claim, _ := sanaei.UUIDv4()
	if _, _, err = service.claimProfileWork(ctx, claim, "TRUE", "l.panel_id"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("leased destination claimed twice", err)
	}
}
