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
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	key := "panel-config:" + panel
	if _, err = conn.ExecContext(ctx, "SELECT pg_advisory_lock(hashtextextended($1,0))", key); err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(c, "SELECT pg_advisory_unlock(hashtextextended($1,0))", key); err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	held := &configLockOwnership{db: db, panel: panel}
	held.active.Store(true)
	defer held.active.Store(false)
	return fn(context.WithValue(ctx, configLockKey{}, held))
}
