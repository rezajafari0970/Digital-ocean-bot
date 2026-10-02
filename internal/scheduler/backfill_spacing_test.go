package scheduler

import (
	"database/sql"
	"testing"
	"time"
)

func TestBackfillBypassesNormalBuildSpacing(t *testing.T) {
	now := time.Now().UTC()
	next := sql.NullTime{Time: now.Add(24 * time.Hour), Valid: true}
	if !buildSpacingBlocked(0, next, now) {
		t.Fatal("normal growth must obey next_build_at")
	}
	if buildSpacingBlocked(1, next, now) {
		t.Fatal("durable lifecycle backfill must bypass normal build spacing")
	}
	if buildSpacingBlocked(0, sql.NullTime{}, now) {
		t.Fatal("missing next_build_at must not block creation")
	}
}

func TestBackfillCanFillMultipleFreeSlotsWithinConcurrencyBudget(t *testing.T) {
	if got := initialAllowedStarts(0, 5, 0, 3); got != 1 {
		t.Fatalf("normal growth got %d; want 1", got)
	}
	if got := initialAllowedStarts(2, 2, 1, 3); got != 2 {
		t.Fatalf("backfill got %d; want 2", got)
	}
	if got := initialAllowedStarts(5, 4, 2, 3); got != 1 {
		t.Fatalf("backfill must respect remaining concurrency slot; got %d", got)
	}
}
