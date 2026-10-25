package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrScheduleLeaseLost = errors.New("schedule claim is no longer owned")

type LeaseStore struct{ DB *sql.DB }

func (s LeaseStore) ClaimDue(ctx context.Context, now time.Time, limit int, lease time.Duration) ([]Schedule, error) {
	if limit < 1 || limit > 100 {
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
	rows, err := tx.QueryContext(ctx, `SELECT id::text,account_id::text,profile_id::text,enabled,interval_seconds,batch_size,max_concurrent,next_run_at,last_run_at FROM schedules WHERE EXISTS(SELECT 1 FROM deployment_profiles p WHERE p.id=schedules.profile_id AND p.account_id=schedules.account_id) AND enabled=true AND next_run_at<=$1 AND (lease_until IS NULL OR lease_until<$1) ORDER BY next_run_at FOR UPDATE SKIP LOCKED LIMIT $2`, now, limit)
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
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range out {
		x := &out[i]
		if err := tx.QueryRowContext(ctx, `UPDATE schedules SET lease_until=$2,lease_token=gen_random_uuid() WHERE id=$1 RETURNING lease_token::text,lease_until`, x.ID, now.Add(lease)).Scan(&x.LeaseToken, &x.LeaseUntil); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// Validate refuses expired, disabled, rebound or superseded work before dispatch.
// Native deployment admission and provider mutation fences remain authoritative.
func (s LeaseStore) Validate(ctx context.Context, x Schedule) error {
	if x.LeaseToken == "" {
		return ErrScheduleLeaseLost
	}
	var owned bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schedules WHERE id=$1 AND account_id=$2 AND profile_id=$3 AND lease_token=$4 AND enabled=true AND lease_until>clock_timestamp() AND EXISTS(SELECT 1 FROM deployment_profiles p WHERE p.id=schedules.profile_id AND p.account_id=schedules.account_id))`, x.ID, x.AccountID, x.ProfileID, x.LeaseToken).Scan(&owned)
	if err != nil {
		return err
	}
	if !owned {
		return ErrScheduleLeaseLost
	}
	return nil
}
func (s LeaseStore) Complete(ctx context.Context, x Schedule, now time.Time) error {
	if x.LeaseToken == "" {
		return ErrScheduleLeaseLost
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE schedules SET last_run_at=$2,next_run_at=$3,lease_until=NULL,lease_token=NULL,updated_at=now() WHERE id=$1 AND account_id=$4 AND profile_id=$5 AND lease_token=$6`, x.ID, now, now.Add(x.Interval), x.AccountID, x.ProfileID, x.LeaseToken)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrScheduleLeaseLost
	}
	return nil
}
