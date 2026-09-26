package provisioning

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrStepRetryDeferred = errors.New("provision step retry deferred")
var ErrStepRetryLimit = errors.New("provision step retry limit reached")
var ErrStepTerminal = errors.New("provision step terminal failure")

type StepPolicy struct{ MaxAttempts int }

var DefaultStepPolicies = map[string]StepPolicy{
	"ssh": {MaxAttempts: 12}, "bootstrap": {MaxAttempts: 5},
	"panel": {MaxAttempts: 5}, "verify": {MaxAttempts: 8},
}

func (s SQLStore) BeginStep(ctx context.Context, runID, step string, max int) (int, error) {
	var attempts int
	var next sql.NullTime
	var terminal bool
	err := s.DB.QueryRowContext(ctx, `SELECT attempts,next_retry_at,terminal FROM provision_step_attempts WHERE run_id=$1 AND step=$2`, runID, step).Scan(&attempts, &next, &terminal)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if err == nil {
		if terminal {
			return attempts, ErrStepTerminal
		}
		if next.Valid && time.Now().Before(next.Time) {
			return attempts, ErrStepRetryDeferred
		}
		if max > 0 && attempts >= max {
			return attempts, ErrStepRetryLimit
		}
	}
	var n int
	err = s.DB.QueryRowContext(ctx, `INSERT INTO provision_step_attempts(run_id,step,attempts,last_started_at,last_error,next_retry_at,terminal) VALUES($1,$2,1,now(),NULL,NULL,false) ON CONFLICT(run_id,step) DO UPDATE SET attempts=provision_step_attempts.attempts+1,last_started_at=now(),last_error=NULL,next_retry_at=NULL RETURNING attempts`, runID, step).Scan(&n)
	return n, err
}

func (s SQLStore) FinishStep(ctx context.Context, runID, step string, stepErr error, terminal bool) error {
	msg := ""
	var next any
	if stepErr != nil {
		msg = stepErr.Error()
		if !terminal {
			var attempts int
			_ = s.DB.QueryRowContext(ctx, `SELECT attempts FROM provision_step_attempts WHERE run_id=$1 AND step=$2`, runID, step).Scan(&attempts)
			if attempts < 1 {
				attempts = 1
			}
			next = time.Now().Add(time.Duration(1<<min(attempts, 6)) * time.Minute)
		}
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE provision_step_attempts SET last_finished_at=now(),last_error=NULLIF($3,''),next_retry_at=$4,terminal=$5 WHERE run_id=$1 AND step=$2`, runID, step, msg, next, terminal)
	return err
}

func (s SQLStore) StepInterrupted(ctx context.Context, runID, step string) (bool, error) {
	var interrupted bool
	err := s.DB.QueryRowContext(ctx, `SELECT last_started_at IS NOT NULL AND (last_finished_at IS NULL OR last_started_at>last_finished_at) FROM provision_step_attempts WHERE run_id=$1 AND step=$2`, runID, step).Scan(&interrupted)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return interrupted, err
}
