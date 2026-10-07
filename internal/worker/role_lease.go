package worker

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"
)

var ErrRoleOwned = errors.New("worker role already owned")

// Session locks prevent duplicate roles and all+split overlap. Existing durable
// operation/account/client locks remain the authority for every remote action.
type RoleLease struct {
	conn            *sql.Conn
	backend         int
	control, panels bool
}

func AcquireRole(ctx context.Context, db *sql.DB, role Role) (*RoleLease, error) {
	if _, err := ParseRole(string(role)); err != nil {
		return nil, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	l := &RoleLease{conn: conn, control: role.Owns(RoleControl), panels: role.Owns(RolePanels)}
	ok := false
	defer func() {
		if !ok {
			l.Close()
		}
	}()
	if err = conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&l.backend); err != nil {
		return nil, err
	}
	// Fixed global order makes acquiring both roles safe.
	for _, group := range []Role{RoleControl, RolePanels} {
		if !role.Owns(group) {
			continue
		}
		var acquired bool
		if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(728391448::int, $1::int)", roleLockOffset(group)).Scan(&acquired); err != nil {
			return nil, err
		}
		if !acquired {
			return nil, fmt.Errorf("%w: %s", ErrRoleOwned, group)
		}
	}
	ok = true
	return l, nil
}
func roleLockOffset(r Role) int {
	if r == RolePanels {
		return 2
	}
	return 1
}
func (l *RoleLease) Check(ctx context.Context) error {
	var backend int
	var control, panels bool
	if err := l.conn.QueryRowContext(ctx, `SELECT pg_backend_pid(),
 EXISTS(SELECT 1 FROM pg_locks WHERE pid=pg_backend_pid() AND locktype='advisory' AND classid=728391448 AND objid=1 AND objsubid=2 AND granted),
 EXISTS(SELECT 1 FROM pg_locks WHERE pid=pg_backend_pid() AND locktype='advisory' AND classid=728391448 AND objid=2 AND objsubid=2 AND granted)`).Scan(&backend, &control, &panels); err != nil {
		return err
	}
	if backend != l.backend || (l.control && !control) || (l.panels && !panels) {
		return errors.New("worker role session changed or lock lost")
	}
	return nil
}
func (l *RoleLease) Run(ctx context.Context) error {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := l.Check(probe)
		cancel()
		if err != nil {
			return fmt.Errorf("worker role ownership lost: %w", err)
		}
	}
}
func (l *RoleLease) Close() {
	if l == nil || l.conn == nil {
		return
	}
	// Discard the dedicated session instead of returning advisory locks to a pool.
	// This releases all its locks even after cancellation or partial acquisition.
	_ = l.conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = l.conn.Close()
}
