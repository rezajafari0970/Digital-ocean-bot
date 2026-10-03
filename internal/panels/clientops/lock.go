package clientops

import (
	"context"
	"database/sql/driver"
	"time"
)

// A pinned connection serializes workers and manual executors across processes.
func (j Journal) executorLock(ctx context.Context) (func(), bool, error) {
	conn, err := j.DB.Conn(ctx)
	if err != nil {
		return nil, false, err
	}
	var ok bool
	if err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(628341902731)`).Scan(&ok); err != nil || !ok {
		conn.Close()
		return nil, false, err
	}
	return func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(c, `SELECT pg_advisory_unlock(628341902731)`); err != nil {
			conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		conn.Close()
	}, true, nil
}
