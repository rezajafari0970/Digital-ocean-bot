package sanaei

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"sync/atomic"
	"time"
)

type configLockKey struct{}
type configLockOwnership struct {
	db     *sql.DB
	panel  string
	active atomic.Bool
}

// ConfigLockHeld verifies the caller still owns the structural mutation lock.
func ConfigLockHeld(ctx context.Context, db *sql.DB, panel string) bool {
	held, ok := ctx.Value(configLockKey{}).(*configLockOwnership)
	return ok && held.db == db && held.panel == panel && held.active.Load()
}

// WithConfigLock serializes structural mutations across API and worker processes.
// Callers take this lock before the shared runtime mutation lock.
func WithConfigLock(ctx context.Context, db *sql.DB, panel string, fn func(context.Context) error) error {
	_, err := withConfigLock(ctx, db, panel, false, fn)
	return err
}

// TryWithConfigLock skips a busy panel without queuing behind structural writes.
// acquired is true once ownership was obtained, even if the callback fails.
func TryWithConfigLock(ctx context.Context, db *sql.DB, panel string, fn func(context.Context) error) (acquired bool, err error) {
	return withConfigLock(ctx, db, panel, true, fn)
}

func withConfigLock(ctx context.Context, db *sql.DB, panel string, try bool, fn func(context.Context) error) (bool, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	key := "panel-config:" + panel
	acquired := false
	if try {
		err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", key).Scan(&acquired)
	} else {
		_, err = conn.ExecContext(ctx, "SELECT pg_advisory_lock(hashtextextended($1,0))", key)
		acquired = err == nil
	}
	if err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return false, err
	}
	if !acquired {
		return false, nil
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var unlocked bool
		if err := conn.QueryRowContext(c, "SELECT pg_advisory_unlock(hashtextextended($1,0))", key).Scan(&unlocked); err != nil || !unlocked {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	held := &configLockOwnership{db: db, panel: panel}
	held.active.Store(true)
	defer held.active.Store(false)
	return true, fn(context.WithValue(ctx, configLockKey{}, held))
}
