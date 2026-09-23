package scheduler

import (
	"context"
	"database/sql"
	"time"
)

type LeaseStore struct{ DB *sql.DB }

func (s LeaseStore) ClaimDue(ctx context.Context, now time.Time, limit int, lease time.Duration) ([]Schedule, error) {
	if limit < 1 {
		limit = 100
	}
	if lease <= 0 {
		lease = time.Minute
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id::text,account_id::text,profile_id::text,enabled,interval_seconds,batch_size,max_concurrent,next_run_at,last_run_at FROM schedules WHERE enabled=true AND next_run_at<=$1 AND (lease_until IS NULL OR lease_until<$1) ORDER BY next_run_at FOR UPDATE SKIP LOCKED LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	var out []Schedule
	for rows.Next() {
		var x Schedule
		var sec int
		var last sql.NullTime
		if err := rows.Scan(&x.ID, &x.AccountID, &x.ProfileID, &x.Enabled, &sec, &x.BatchSize, &x.MaxConcurrent, &x.NextRunAt, &last); err != nil {
			rows.Close()
			return nil, err
		}
		x.Interval = time.Duration(sec) * time.Second
		if last.Valid {
			x.LastRunAt = &last.Time
		}
		out = append(out, x)
	}
	rows.Close()
	for _, x := range out {
		if _, err := tx.ExecContext(ctx, `UPDATE schedules SET lease_until=$2 WHERE id=$1`, x.ID, now.Add(lease)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}
func (s LeaseStore) Complete(ctx context.Context, x Schedule, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE schedules SET last_run_at=$2,next_run_at=$3,lease_until=NULL,updated_at=now() WHERE id=$1`, x.ID, now, now.Add(x.Interval))
	return err
}
