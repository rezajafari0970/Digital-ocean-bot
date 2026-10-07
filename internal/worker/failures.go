package worker

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
	"time"
	"unicode/utf8"
)

type FailureStore struct{ DB *sql.DB }

func (s FailureStore) Due(ctx context.Context, kind, itemID string) bool {
	due, err := s.DueChecked(ctx, kind, itemID)
	return err == nil && due
}
func (s FailureStore) DueChecked(ctx context.Context, kind, itemID string) (bool, error) {
	if s.DB == nil {
		return true, nil
	}
	var next sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT next_retry_at FROM worker_item_failures WHERE kind=$1 AND item_id=$2`, kind, itemID).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return !next.Valid || !time.Now().Before(next.Time), nil
}

func (s FailureStore) ReserveOutcome(ctx context.Context, kind, item, account string, hold time.Duration) (bool, error) {
	if s.DB == nil || hold < time.Second || hold > 10*time.Minute {
		return false, errors.New("invalid outcome reservation")
	}
	var claimed bool
	err := s.DB.QueryRowContext(ctx, `INSERT INTO worker_item_failures(kind,item_id,account_id,failures,last_error,first_failed_at,last_failed_at,next_retry_at)
 VALUES($1,$2,NULLIF($3,'')::uuid,0,'RECONCILIATION_OUTCOME_PENDING',now(),now(),now()+($4*interval '1 second'))
 ON CONFLICT(kind,item_id) DO UPDATE SET next_retry_at=EXCLUDED.next_retry_at
 WHERE worker_item_failures.next_retry_at IS NULL OR worker_item_failures.next_retry_at<=now()
 RETURNING true`, kind, item, account, int64(hold/time.Second)).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return claimed, err
}

// RecordOutcome reports essential writes and enforces cancellation-aware pacing
// when no durable retry can be stored. Native ambiguity guards remain separate.
func (s FailureStore) RecordOutcome(ctx context.Context, kind, item, account string, cause error) error {
	var err error
	if cause != nil {
		err = s.FailChecked(ctx, kind, item, account, cause)
	} else {
		err = s.ClearChecked(ctx, kind, item)
	}
	if err == nil {
		return nil
	}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	case <-timer.C:
		return err
	}
}
func (s FailureStore) Fail(ctx context.Context, kind, itemID, accountID string, cause error) {
	if err := s.FailChecked(ctx, kind, itemID, accountID, cause); err != nil {
		log.Printf("worker failure ledger write failed: %v", err)
	}
}
func (s FailureStore) FailChecked(ctx context.Context, kind, itemID, accountID string, cause error) error {
	if s.DB == nil || cause == nil {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = failTx(ctx, tx, kind, itemID, accountID, cause); err != nil {
		return err
	}
	return tx.Commit()
}
func failTx(ctx context.Context, tx *sql.Tx, kind, itemID, accountID string, cause error) error {
	msg := ledgerErrorText(cause)
	var failures int
	err := tx.QueryRowContext(ctx, `INSERT INTO worker_item_failures(kind,item_id,account_id,failures,last_error,first_failed_at,last_failed_at) VALUES($1,$2,NULLIF($3,'')::uuid,1,$4,now(),now()) ON CONFLICT(kind,item_id) DO UPDATE SET failures=LEAST(worker_item_failures.failures+1,1000000),last_error=EXCLUDED.last_error,last_failed_at=now() RETURNING failures`, kind, itemID, accountID, msg).Scan(&failures)
	if err != nil {
		return err
	}
	delay := time.Duration(1<<minFailure(failures, 6)) * time.Second
	if hinted, ok := boundedRetryDelay(cause); ok {
		delay = hinted
	}
	seconds := int((delay + time.Second - 1) / time.Second)
	_, err = tx.ExecContext(ctx, `UPDATE worker_item_failures SET next_retry_at=now()+($3 * interval '1 second') WHERE kind=$1 AND item_id=$2`, kind, itemID, seconds)
	if err != nil {
		return err
	}
	return nil
}

// PostgreSQL text rejects NUL and invalid UTF-8. Keep a valid bounded suffix.
func ledgerErrorText(cause error) string {
	if cause == nil {
		return ""
	}
	msg := strings.ReplaceAll(strings.ToValidUTF8(cause.Error(), "�"), "\x00", "�")
	if len(msg) > 2048 {
		msg = msg[len(msg)-2048:]
		for len(msg) > 0 && !utf8.RuneStart(msg[0]) {
			msg = msg[1:]
		}
	}
	return msg
}
func clearFailureTx(ctx context.Context, tx *sql.Tx, kind, itemID string) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM worker_item_failures WHERE kind=$1 AND item_id=$2", kind, itemID)
	return err
}

func (s FailureStore) Clear(ctx context.Context, kind, itemID string) {
	if err := s.ClearChecked(ctx, kind, itemID); err != nil {
		log.Printf("worker failure ledger clear failed: %v", err)
	}
}
func (s FailureStore) ClearChecked(ctx context.Context, kind, itemID string) error {
	if s.DB == nil {
		return nil
	}
	_, err := s.DB.ExecContext(ctx, "DELETE FROM worker_item_failures WHERE kind=$1 AND item_id=$2", kind, itemID)
	return err
}

func (s FailureStore) ClearResolved(ctx context.Context) { _ = s.ClearResolvedChecked(ctx) }
func (s FailureStore) ClearResolvedChecked(ctx context.Context) error {
	if s.DB == nil {
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM worker_item_failures f WHERE
 (f.kind='deployment' AND EXISTS(SELECT 1 FROM deployments d WHERE d.id::text=f.item_id AND (d.state IN ('READY','FAILED','INSTALL_COMPLETE','INSTALL_FAILED','INSTALL_ROLLED_BACK') OR (d.state='WAITING_INSTALLER' AND d.profile_snapshot->'installer_ref' IS NULL AND NOT EXISTS(SELECT 1 FROM deployment_installer_selections s WHERE s.deployment_id=d.id AND s.generation=d.installer_generation)))))
 OR (f.kind='operation' AND EXISTS(SELECT 1 FROM operations o WHERE o.id::text=f.item_id AND o.state IN ('succeeded','failed')))
 OR (f.kind='lifecycle' AND EXISTS(SELECT 1 FROM droplets d WHERE d.id::text=f.item_id AND d.state='DELETED'))`)
	return err
}

// RetryDelayHint marks an error with a preferred delay before worker recovery.
type RetryDelayHint interface {
	RetryDelay() time.Duration
}

const MaxRetryDelayHint = 10 * time.Minute

func boundedRetryDelay(err error) (time.Duration, bool) {
	var hint RetryDelayHint
	if !errors.As(err, &hint) {
		return 0, false
	}
	delay := hint.RetryDelay()
	if delay <= 0 || delay > MaxRetryDelayHint {
		return 0, false
	}
	return delay, true
}

func minFailure(a, b int) int {
	if a < b {
		return a
	}
	return b
}
