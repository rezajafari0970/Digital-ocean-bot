package scheduler

import (
	"context"
	"database/sql"
	"time"
)

type Schedule struct {
	// Returned only by ClaimDue; terminal writes require this exact claim.
	LeaseToken    string
	LeaseUntil    time.Time
	ID            string
	AccountID     string
	ProfileID     string
	Enabled       bool
	Interval      time.Duration
	BatchSize     int
	MaxConcurrent int
	NextRunAt     time.Time
	LastRunAt     *time.Time
}
type SQLStore struct{ DB *sql.DB }

func (s SQLStore) Due(ctx context.Context, now time.Time, limit int) ([]Schedule, error) {
	if limit < 1 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id::text,account_id::text,profile_id::text,enabled,interval_seconds,batch_size,max_concurrent,next_run_at,last_run_at FROM schedules WHERE enabled=true AND next_run_at<=$1 ORDER BY next_run_at FOR UPDATE SKIP LOCKED LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Schedule
	for rows.Next() {
		var x Schedule
		var sec int
		var last sql.NullTime
		if err := rows.Scan(&x.ID, &x.AccountID, &x.ProfileID, &x.Enabled, &sec, &x.BatchSize, &x.MaxConcurrent, &x.NextRunAt, &last); err != nil {
			return nil, err
		}
		x.Interval = time.Duration(sec) * time.Second
		if last.Valid {
			x.LastRunAt = &last.Time
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// Advance shares the owned completion path; a read-only Due result is not a claim.
func (s SQLStore) Advance(ctx context.Context, x Schedule, now time.Time) error {
	return (LeaseStore{DB: s.DB}).Complete(ctx, x, now)
}
