package worker

import (
	"context"
	"database/sql"
	"time"
)

type FailureStore struct{ DB *sql.DB }

func (s FailureStore) Due(ctx context.Context, kind, itemID string) bool {
	if s.DB == nil {
		return true
	}
	var next sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT next_retry_at FROM worker_item_failures WHERE kind=$1 AND item_id=$2`, kind, itemID).Scan(&next)
	if err != nil || !next.Valid {
		return true
	}
	return !time.Now().Before(next.Time)
}

func (s FailureStore) Fail(ctx context.Context, kind, itemID, accountID string, err error) {
	if s.DB == nil || err == nil {
		return
	}
	msg := err.Error()
	if len(msg) > 2048 {
		msg = msg[len(msg)-2048:]
	}
	var failures int
	_ = s.DB.QueryRowContext(ctx, `INSERT INTO worker_item_failures(kind,item_id,account_id,failures,last_error,first_failed_at,last_failed_at)
VALUES($1,$2,NULLIF($3,'')::uuid,1,$4,now(),now())
ON CONFLICT(kind,item_id) DO UPDATE SET failures=worker_item_failures.failures+1,last_error=EXCLUDED.last_error,last_failed_at=now()
RETURNING failures`, kind, itemID, accountID, msg).Scan(&failures)
	if failures < 1 {
		failures = 1
	}
	backoff := time.Duration(1<<minFailure(failures, 6)) * time.Second
	_, _ = s.DB.ExecContext(ctx, `UPDATE worker_item_failures SET next_retry_at=now()+($3 * interval '1 second') WHERE kind=$1 AND item_id=$2`, kind, itemID, int(backoff/time.Second))
}

func (s FailureStore) Clear(ctx context.Context, kind, itemID string) {
	if s.DB == nil {
		return
	}
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM worker_item_failures WHERE kind=$1 AND item_id=$2`, kind, itemID)
}

func (s FailureStore) ClearResolved(ctx context.Context) {
	if s.DB == nil {
		return
	}
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM worker_item_failures f WHERE
 (f.kind='deployment' AND EXISTS(SELECT 1 FROM deployments d WHERE d.id::text=f.item_id AND d.state IN ('READY','FAILED')))
 OR (f.kind='operation' AND EXISTS(SELECT 1 FROM operations o WHERE o.id::text=f.item_id AND o.state IN ('succeeded','failed')))
 OR (f.kind='lifecycle' AND EXISTS(SELECT 1 FROM droplets d WHERE d.id::text=f.item_id AND d.state='DELETED'))`)
}

func minFailure(a, b int) int {
	if a < b {
		return a
	}
	return b
}
