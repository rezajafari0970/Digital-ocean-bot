package worker

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log"
	"time"
)

var ErrRecoveryCheckpointPending = errors.New("recovery checkpoint requires persisted reconciliation")
var ErrRecoveryCheckpointLost = errors.New("recovery checkpoint ownership lost")

type recoveryInterrupted struct{}

func (recoveryInterrupted) Error() string             { return "RECOVERY_INTERRUPTED_BEFORE_DURABLE_COMPLETION" }
func (recoveryInterrupted) RetryDelay() time.Duration { return 30 * time.Second }

// This session fence complements the role owner and native operation leases.
// The checkpoint survives process/session loss; no remote handler runs during
// orphan reconciliation. One extra connection is included in control admission.
type recoveryLease struct {
	conn                     *sql.Conn
	key, kind, item, attempt string
	backend                  int
	accountLocked            string
}

func acquireRecoveryLease(ctx context.Context, db *sql.DB, kind, item string) (*recoveryLease, bool, error) {
	if db == nil {
		return nil, false, errors.New("recovery checkpoint database required")
	}
	c, err := db.Conn(ctx)
	if err != nil {
		return nil, false, err
	}
	l := &recoveryLease{conn: c, kind: kind, item: item, key: "worker-recovery:" + kind + ":" + item}
	ok := false
	defer func() {
		if !ok {
			l.close()
		}
	}()
	var locked bool
	if err = c.QueryRowContext(ctx, "SELECT pg_backend_pid(),pg_try_advisory_lock(hashtextextended($1,728391449))", l.key).Scan(&l.backend, &locked); err != nil {
		return nil, false, err
	}
	if !locked {
		return nil, false, nil
	}
	ok = true
	return l, true, nil
}
func (l *recoveryLease) close() {
	if l == nil || l.conn == nil {
		return
	}
	_ = l.conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = l.conn.Close()
}
func (l *recoveryLease) check(ctx context.Context) error {
	var owns bool
	err := l.conn.QueryRowContext(ctx, `SELECT pg_backend_pid()=$2 AND EXISTS(
 SELECT 1 FROM pg_locks WHERE pid=pg_backend_pid() AND locktype='advisory' AND objsubid=1 AND granted
 AND classid::bigint=((hashtextextended($1,728391449)>>32)&4294967295)
 AND objid::bigint=(hashtextextended($1,728391449)&4294967295))`, l.key, l.backend).Scan(&owns)
	if err != nil {
		return err
	}
	if !owns {
		return ErrRecoveryCheckpointLost
	}
	if l.accountLocked != "" {
		err = l.conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=pg_backend_pid() AND locktype='advisory' AND objsubid=1 AND granted AND mode='ShareLock' AND classid::bigint=((hashtextextended($1,728391450)>>32)&4294967295) AND objid::bigint=(hashtextextended($1,728391450)&4294967295))`, "worker-recovery-account:"+l.accountLocked).Scan(&owns)
		if err != nil {
			return err
		}
		if !owns {
			return ErrRecoveryCheckpointLost
		}
	}
	return nil
}

var ErrRecoveryAccountBusy = errors.New("account recovery ownership busy")

// Purge never waits behind a recovery handler while holding account/native locks.
func FenceRecoveryAccountPurge(ctx context.Context, tx *sql.Tx, account string) error {
	var ok bool
	if err := tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock(hashtextextended($1,728391450))", "worker-recovery-account:"+account).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrRecoveryAccountBusy
	}
	var pending bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM worker_recovery_checkpoints WHERE account_id=$1)", account).Scan(&pending); err != nil {
		return err
	}
	// Session loss is not proof that an in-process native handler stopped.
	// Only the owning reconciler can close a pending checkpoint before purge.
	if pending {
		return ErrRecoveryCheckpointPending
	}
	return nil
}
func (l *recoveryLease) lockAccount(ctx context.Context, account string) error {
	if l.accountLocked == account {
		return nil
	}
	var ok bool
	if err := l.conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock_shared(hashtextextended($1,728391450))", "worker-recovery-account:"+account).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrRecoveryAccountBusy
	}
	l.accountLocked = account
	return nil
}
func (l *recoveryLease) begin(ctx context.Context, account string) error {
	if err := l.lockAccount(ctx, account); err != nil {
		return err
	}
	var exists bool
	if err := l.conn.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1)", account).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return sql.ErrNoRows
	}
	if err := l.check(ctx); err != nil {
		return err
	}
	err := l.conn.QueryRowContext(ctx, `INSERT INTO worker_recovery_checkpoints(kind,item_id,account_id)
 VALUES($1,$2,$3) ON CONFLICT(kind,item_id) DO NOTHING RETURNING attempt_id::text`, l.kind, l.item, account).Scan(&l.attempt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRecoveryCheckpointPending
	}
	return err
}
func (l *recoveryLease) complete(ctx context.Context, account string, cause error) error {
	if err := l.lockAccount(ctx, account); err != nil {
		return err
	}
	if err := l.check(ctx); err != nil {
		return err
	}
	tx, err := l.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var token string
	if err = tx.QueryRowContext(ctx, "SELECT attempt_id::text FROM worker_recovery_checkpoints WHERE kind=$1 AND item_id=$2 FOR UPDATE", l.kind, l.item).Scan(&token); err != nil {
		return err
	}
	if token != l.attempt {
		return ErrRecoveryCheckpointLost
	}
	var accountExists bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1)", account).Scan(&accountExists); err != nil {
		return err
	}
	// Account removal is explicit terminal evidence; never invent a failure FK.
	if cause != nil && accountExists {
		err = failTx(ctx, tx, l.kind, l.item, account, cause)
	} else {
		err = clearFailureTx(ctx, tx, l.kind, l.item)
	}
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, "DELETE FROM worker_recovery_checkpoints WHERE kind=$1 AND item_id=$2 AND attempt_id=$3", l.kind, l.item, l.attempt)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrRecoveryCheckpointLost
	}
	return tx.Commit()
}

type pendingRecovery struct{ kind, item, account string }

// Called before discovery. Active in-process lanes are skipped even if their
// SQL session has failed. Cross-process live sessions are skipped by try-lock.
// Failed items remain fenced; healthy items can be discovered independently.
func reconcileRecoveryCheckpoints(ctx context.Context, db *sql.DB, active func(string) bool, module string, cursor ...*pendingRecovery) error {
	after := pendingRecovery{}
	if len(cursor) > 0 {
		after = *cursor[0]
	}
	rows, err := db.QueryContext(ctx, "SELECT kind,item_id,account_id::text FROM worker_recovery_checkpoints WHERE (($1='lifecycle' AND kind='lifecycle') OR ($1='recovery' AND kind IN ('operation','deployment'))) AND reconcile_after<=now() ORDER BY CASE WHEN (kind,item_id)>($2,$3) THEN 0 ELSE 1 END,kind,item_id LIMIT 100", module, after.kind, after.item)
	if err != nil {
		return err
	}
	items := []pendingRecovery{}
	for rows.Next() {
		var x pendingRecovery
		if err = rows.Scan(&x.kind, &x.item, &x.account); err != nil {
			rows.Close()
			return err
		}
		items = append(items, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var pending error
	reaperEnd := time.Now().Add(2 * time.Second)
	for _, x := range items {
		if time.Now().After(reaperEnd) {
			pending = errors.Join(pending, ErrRecoveryCheckpointPending)
			break
		}
		// Advance even when ownership, native completion, or retry diagnostics fail.
		if len(cursor) > 0 {
			*cursor[0] = x
		}
		if active != nil && active(x.kind+":"+x.item) {
			continue
		}
		itemEnd := time.Now().Add(250 * time.Millisecond)
		if reaperEnd.Before(itemEnd) {
			itemEnd = reaperEnd
		}
		itemCtx, itemCancel := context.WithDeadline(ctx, itemEnd)
		err = func() error {
			ctx := itemCtx
			l, ok, e := acquireRecoveryLease(ctx, db, x.kind, x.item)
			if e != nil || !ok {
				return e
			}
			defer l.close()
			e = l.conn.QueryRowContext(ctx, "SELECT attempt_id::text FROM worker_recovery_checkpoints WHERE kind=$1 AND item_id=$2", x.kind, x.item).Scan(&l.attempt)
			if errors.Is(e, sql.ErrNoRows) {
				return nil
			}
			if e != nil {
				return e
			}
			return l.complete(ctx, x.account, recoveryInterrupted{})
		}()
		itemCancel()
		if err != nil {
			pending = errors.Join(pending, fmt.Errorf("recovery checkpoint reconcile: %w", err))
			// A durable per-item delay rotates poison rows out of the reaper window.
			// Failure of this diagnostic write never removes the original checkpoint.
			writeCtx, writeCancel := context.WithTimeout(ctx, 250*time.Millisecond)
			_, writeErr := db.ExecContext(writeCtx, "UPDATE worker_recovery_checkpoints SET reconcile_after=now()+interval '30 seconds',reconcile_error=$3 WHERE kind=$1 AND item_id=$2", x.kind, x.item, ledgerErrorText(err))
			writeCancel()
			pending = errors.Join(pending, writeErr)
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var delayed bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM worker_recovery_checkpoints WHERE (($1='lifecycle' AND kind='lifecycle') OR ($1='recovery' AND kind IN ('operation','deployment'))) AND reconcile_error<>'')", module).Scan(&delayed); err != nil {
		return err
	}
	if pending != nil || delayed || len(items) == 100 {
		return errors.Join(ErrRecoveryCheckpointPending, pending)
	}
	return nil
}

func runCheckpointedRecovery(ctx context.Context, db *sql.DB, kind string, x RecoveryItem, dueCheck func(context.Context) (bool, error), handler func(context.Context) error, admit func(context.Context, func(context.Context) error) error) error {
	run := func(ctx context.Context) error {
		admitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		l, ok, err := acquireRecoveryLease(admitCtx, db, kind, x.ID)
		if err == nil && ok {
			var due bool
			due, err = dueCheck(admitCtx)
			if err == nil && !due {
				l.close()
				cancel()
				return nil
			}
			if err == nil {
				err = l.begin(admitCtx, x.AccountID)
			}
		}
		cancel()
		if l != nil {
			defer l.close()
		}
		if err != nil || !ok {
			return err
		}
		// Never enter a handler after cancellation or an unproven checkpoint.
		if err = ctx.Err(); err == nil {
			err = l.check(ctx)
		}
		if err == nil {
			err = handler(ctx)
		}
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer finishCancel()
		if persistErr := l.complete(finishCtx, x.AccountID, err); persistErr != nil {
			return fmt.Errorf("durable recovery completion: %w", persistErr)
		}
		if err != nil && ctx.Err() == nil {
			log.Printf("recovery %s %s account=%s: %v", kind, x.ID, x.AccountID, err)
		}
		return nil
	}
	if admit != nil {
		return admit(ctx, run)
	}
	return run(ctx)
}
