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
