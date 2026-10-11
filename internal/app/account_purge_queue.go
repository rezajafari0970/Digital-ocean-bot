package app

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"time"
)

// QueueSavedAccountPurge durably records the operator's exact local-only
// acknowledgement. It authorizes no cloud mutation and never drops native
// recovery evidence. The existing purge fences are acquired during execution.
func QueueSavedAccountPurge(ctx context.Context, db *sql.DB, id, state string, remaining int, confirmation AccountPurgeConfirmation) error {
	if !canonicalPurgeAccountID(id) || !localPurgeAllowed(state) || remaining < 0 {
		return ErrAccountPurgeConflict
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SET LOCAL lock_timeout='2s'"); err != nil {
		return err
	}
	var current string
	var safe bool
	var live int
	var scope string
	var requested sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT provider_state,
 COALESCE(NOT enabled AND deletion_requested_at<now()-interval '2 minutes' AND runtime_status='DELETE_PENDING',false),
 (SELECT count(*) FROM (SELECT provider_resource_id FROM droplets WHERE account_id=a.id AND state<>'DELETED' UNION SELECT provider_resource_id FROM resources WHERE account_id=a.id AND managed AND state<>'deleted') remaining),
 deletion_requested_at,`+accountPurgeScopeSQL+` FROM accounts a WHERE id=$1 FOR UPDATE`, id).Scan(&current, &safe, &live, &requested, &scope)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !confirmation.matches(requested, scope) || !safe || !requested.Valid || current != state || live != remaining {
		return ErrAccountPurgeConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO account_purge_requests(account_id,expected_provider_state,expected_remaining_servers,deletion_requested_at,expected_resource_scope)
 VALUES($1,$2,$3,$4,$5) ON CONFLICT(account_id) DO UPDATE SET
 expected_provider_state=EXCLUDED.expected_provider_state,expected_remaining_servers=EXCLUDED.expected_remaining_servers,
 deletion_requested_at=EXCLUDED.deletion_requested_at,expected_resource_scope=EXCLUDED.expected_resource_scope,acknowledged_at=now(),next_attempt_at=now(),state='QUEUED',last_error=''`, id, state, remaining, confirmation.RequestedAt, confirmation.ResourceScope)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ProcessSavedAccountPurge executes at most one acknowledged request per tick.
// A separate session lease avoids a second owner, without locking the queue
// row across the purge transaction (its FK cascades on final account removal).
func (c Container) ProcessSavedAccountPurge(ctx context.Context) (bool, error) {
	var after any
	lastID := ""
	for {
		rows, err := c.DB.QueryContext(ctx, `SELECT account_id::text,next_attempt_at FROM account_purge_requests
 WHERE state<>'NEEDS_REVIEW' AND next_attempt_at<=now()
 AND ($1::timestamptz IS NULL OR (next_attempt_at,account_id)>($1::timestamptz,NULLIF($2,'')::uuid))
 ORDER BY next_attempt_at,account_id LIMIT 32`, after, lastID)
		if err != nil {
			return false, err
		}
		type candidate struct {
			id  string
			due time.Time
		}
		var ids []candidate
		for rows.Next() {
			var item candidate
			if err = rows.Scan(&item.id, &item.due); err != nil {
				rows.Close()
				return false, err
			}
			ids = append(ids, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return false, err
		}
		if len(ids) == 0 {
			return false, nil
		}
		for _, item := range ids {
			id := item.id
			after = item.due
			lastID = id
			conn, err := c.DB.Conn(ctx)
			if err != nil {
				return false, err
			}
			var locked bool
			err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,714))", id).Scan(&locked)
			if err != nil {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
				conn.Close()
				return false, err
			}
			if !locked {
				conn.Close()
				continue
			}
			result := func() error {
				defer func() {
					release, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					if _, e := conn.ExecContext(release, "SELECT pg_advisory_unlock(hashtextextended($1,714))", id); e != nil {
						_ = conn.Raw(func(any) error { return driver.ErrBadConn })
					}
					conn.Close()
				}()
				var state, scope string
				var remaining int
				var requested, acknowledged time.Time
				err := conn.QueryRowContext(ctx, `UPDATE account_purge_requests SET attempts=attempts+1,next_attempt_at=now()+interval '1 minute'
 WHERE account_id=$1 AND state<>'NEEDS_REVIEW' AND next_attempt_at<=now()
 RETURNING expected_provider_state,expected_remaining_servers,deletion_requested_at,acknowledged_at,expected_resource_scope`, id).Scan(&state, &remaining, &requested, &acknowledged, &scope)
				if errors.Is(err, sql.ErrNoRows) {
					return nil
				}
				if err != nil {
					return err
				}
				work, cancel := context.WithTimeout(ctx, 30*time.Second)
				err = PurgeSavedAccount(work, c.DB, id, state, remaining, AccountPurgeConfirmation{RequestedAt: requested, ResourceScope: scope})
				cancel()
				if err == nil {
					return nil
				} // Queue row was cascaded in the same commit.
				status, detail := "RETRY", "Waiting for account-scoped work or cleanup; automatic retry is scheduled."
				if errors.Is(err, ErrAccountPurgeConflict) && !errors.Is(err, worker.ErrRecoveryAccountBusy) && !errors.Is(err, worker.ErrRecoveryCheckpointPending) {
					status = "NEEDS_REVIEW"
					detail = "Account or remaining resources changed. Refresh and confirm local removal again."
				}
				save, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
				defer stop()
				_, saveErr := c.DB.ExecContext(save, `UPDATE account_purge_requests SET state=$2,last_error=$3,next_attempt_at=now()+(LEAST(300,5*GREATEST(attempts,1))*interval '1 second') WHERE account_id=$1 AND acknowledged_at=$4`, id, status, detail, acknowledged)
				if saveErr != nil {
					return errors.Join(err, fmt.Errorf("persist local purge progress: %w", saveErr))
				}
				return err
			}()
			return true, result
		}
	}
}
