package worker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOwnedDeferralPreservesFailuresAndReplacementClaim(t *testing.T) {
	db := failureLedgerFixture(t)
	s := FailureStore{DB: db}
	ctx := context.Background()
	if e := s.FailChecked(ctx, "route", "one", "", errors.New("actual native fault")); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec("UPDATE worker_item_failures SET next_retry_at=now()-interval '1 second'"); e != nil {
		t.Fatal(e)
	}
	first, ok, e := s.ReserveOwnedOutcome(ctx, "route", "one", "", 90*time.Second)
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
	if e = s.DeferOwnedOutcome(ctx, "route", "one", first, 2*time.Second); e != nil {
		t.Fatal(e)
	}
	var failures int
	var message string
	var next time.Time
	if e = db.QueryRow("SELECT failures,last_error,next_retry_at FROM worker_item_failures").Scan(&failures, &message, &next); e != nil || failures != 1 || message != "actual native fault" || time.Until(next) > 3*time.Second || time.Until(next) < time.Second {
		t.Fatal(failures, message, next, e)
	}
	if _, e = db.Exec("UPDATE worker_item_failures SET next_retry_at=now()-interval '1 second'"); e != nil {
		t.Fatal(e)
	}
	replacement, ok, e := s.ReserveOwnedOutcome(ctx, "route", "one", "", 90*time.Second)
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
	if e = s.DeferOwnedOutcome(ctx, "route", "one", first, 2*time.Second); e != nil {
		t.Fatal(e)
	}
	if e = db.QueryRow("SELECT next_retry_at FROM worker_item_failures").Scan(&next); e != nil || !next.Equal(replacement.Version) {
		t.Fatal("stale owner changed replacement", next, e)
	}
	if _, e = db.Exec("CREATE FUNCTION reject_deferral() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'write fault'; END$$; CREATE TRIGGER reject_deferral BEFORE UPDATE ON worker_item_failures FOR EACH ROW EXECUTE FUNCTION reject_deferral()"); e != nil {
		t.Fatal(e)
	}
	if e = s.DeferOwnedOutcome(ctx, "route", "one", replacement, 2*time.Second); e == nil {
		t.Fatal("deferral write failure hidden")
	}
	if e = s.DeferOwnedOutcome(ctx, "route", "one", replacement, 0); e == nil {
		t.Fatal("unbounded retry delay accepted")
	}
}
