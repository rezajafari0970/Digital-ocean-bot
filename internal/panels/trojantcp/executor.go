package trojantcp

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"
)

// Queue on the same single global executor used by existing native mutations.
// The caller holds no native configuration lock while waiting. Cancellation
// cleanup also handles an acquired lock whose acknowledgement was lost.
func acquireCreationExecutor(ctx context.Context, conn *sql.Conn) (func(), error) {
	release := func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var unlocked bool
		if err := conn.QueryRowContext(cleanup, "SELECT pg_advisory_unlock(628341902731)").Scan(&unlocked); err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := conn.ExecContext(bounded, "SELECT pg_advisory_lock(628341902731)")
	if err != nil {
		if ctx.Err() != nil {
			return release, ctx.Err()
		}
		return release, errors.New("trojan_executor_busy")
	}
	return release, nil
}
