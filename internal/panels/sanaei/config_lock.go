package sanaei

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type configLockKey struct{}

const (
	configMutation int32 = iota + 1
	configProof
	configObservation
)

var ErrConfigOwnership = errors.New("panel configuration ownership unavailable")

type configAdvisory struct {
	key    string
	shared bool
}
type configLockOwnership struct {
	db     *sql.DB
	panel  string
	conn   *sql.Conn
	ctx    context.Context
	cancel context.CancelFunc
	active atomic.Bool
	mode   atomic.Int32
	mu     sync.Mutex // Serialize evidence transactions, downgrade and lease probes.
	locks  []configAdvisory
}

// ConfigObservationEnabled is opt-in during the writer-complete rollout. All
// processes must acquire both writer fences before any reader scope is enabled.
func ConfigObservationEnabled(panel string) bool {
	for _, id := range strings.Split(os.Getenv("DOB_CONFIG_OBSERVATION_PANELS"), ",") {
		if v := strings.TrimSpace(id); v == "all" || (panel != "" && v == panel) {
			return true
		}
	}
	return false
}
func configOwner(ctx context.Context, db *sql.DB, panel string) (*configLockOwnership, bool) {
	h, ok := ctx.Value(configLockKey{}).(*configLockOwnership)
	return h, ok && h.db == db && h.panel == panel && h.active.Load() && h.ctx.Err() == nil
}

// ConfigLockHeld grants mutation authority only while both exclusive fences
// remain owned. A previously captured context loses authority on downgrade.
func ConfigLockHeld(ctx context.Context, db *sql.DB, panel string) bool {
	h, ok := configOwner(ctx, db, panel)
	return ok && h.mode.Load() == configMutation
}
func WithConfigLock(ctx context.Context, db *sql.DB, panel string, fn func(context.Context) error) error {
	_, err := withConfigLock(ctx, db, panel, false, false, fn)
	return err
}
func TryWithConfigLock(ctx context.Context, db *sql.DB, panel string, fn func(context.Context) error) (bool, error) {
	return withConfigLock(ctx, db, panel, true, false, fn)
}

// TryWithConfigReadLock observes native configuration without acquiring the
// turnstile. A proof keeps the turnstile, so a queued writer cannot block reads
// until that proof has finished. A waiting observation-barrier writer then
// prevents new readers from indefinitely extending its wait.
func TryWithConfigReadLock(ctx context.Context, db *sql.DB, panel string, fn func(context.Context) error) (bool, error) {
	return withConfigLock(ctx, db, panel, true, ConfigObservationEnabled(panel), fn)
}

// ConfigProofOnly is irreversible: it retains the exclusive turnstile and
// runtime mutation mutex while sharing only the observation barrier. Callers
// must never invoke update/restart fallback after this transition.
func ConfigProofOnly(ctx context.Context, db *sql.DB, panel string) error {
	h, ok := configOwner(ctx, db, panel)
	if !ok || !ConfigObservationEnabled(panel) {
		return ErrConfigOwnership
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.active.Load() || h.ctx.Err() != nil || h.mode.Load() != configMutation {
		return ErrConfigOwnership
	}
	key := "panel-config-observe:" + panel
	if _, err := h.conn.ExecContext(ctx, "SELECT pg_advisory_lock_shared(hashtextextended($1,0))", key); err != nil {
		h.invalidate()
		// A session lock may have been granted before an SQL error reached us.
		// Discard even when unlocking the recorded locks would succeed.
		_ = h.conn.Raw(func(any) error { return driver.ErrBadConn })
		return err
	}
	h.locks = append(h.locks, configAdvisory{key, true})
	h.mode.Store(configProof)
	var unlocked bool
	if err := h.conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock(hashtextextended($1,0))", key).Scan(&unlocked); err != nil || !unlocked {
		h.invalidate()
		// A session lock may have been granted before an SQL error reached us.
		// Discard even when unlocking the recorded locks would succeed.
		_ = h.conn.Raw(func(any) error { return driver.ErrBadConn })
		return ErrConfigOwnership
	}
	h.locks = []configAdvisory{h.locks[0], h.locks[2]}
	return nil
}

// WithConfigEvidenceTx publishes evidence on the same physical PostgreSQL
// session that owns its fence. A separate pooled connection, even after a
// successful heartbeat, cannot guarantee that ownership survives publication.
func WithConfigEvidenceTx(ctx context.Context, db *sql.DB, panel string, fn func(*sql.Tx) error) error {
	h, ok := configOwner(ctx, db, panel)
	if !ok {
		return ErrConfigOwnership
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.active.Load() || h.ctx.Err() != nil {
		return ErrConfigOwnership
	}
	tx, err := h.conn.BeginTx(ctx, nil)
	if err != nil {
		h.invalidate()
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
func (h *configLockOwnership) invalidate() {
	h.active.Store(false)
	h.cancel()
}
func withConfigLock(ctx context.Context, db *sql.DB, panel string, try, read bool, fn func(context.Context) error) (bool, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	work, cancel := context.WithCancel(ctx)
	h := &configLockOwnership{db: db, panel: panel, conn: conn, ctx: work, cancel: cancel}
	defer cancel()
	// Always remove every partially acquired lock. Uncertain ownership discards
	// the session rather than returning a potentially locked session to the pool.
	defer func() {
		h.active.Store(false)
		c, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		for i := len(h.locks) - 1; i >= 0; i-- {
			l := h.locks[i]
			q := "SELECT pg_advisory_unlock(hashtextextended($1,0))"
			if l.shared {
				q = "SELECT pg_advisory_unlock_shared(hashtextextended($1,0))"
			}
			var released bool
			if e := conn.QueryRowContext(c, q, l.key).Scan(&released); e != nil || !released {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
				return
			}
		}
	}()
	wanted := []configAdvisory{{"panel-config:" + panel, false}, {"panel-config-observe:" + panel, false}}
	h.mode.Store(configMutation)
	if read {
		wanted = []configAdvisory{{"panel-config-observe:" + panel, true}}
		h.mode.Store(configObservation)
	}
	for _, l := range wanted {
		name := "pg_advisory_lock"
		if try {
			name = "pg_try_advisory_lock"
		}
		if l.shared {
			name += "_shared"
		}
		acquired := false
		if try {
			err = conn.QueryRowContext(ctx, "SELECT "+name+"(hashtextextended($1,0))", l.key).Scan(&acquired)
		} else {
			_, err = conn.ExecContext(ctx, "SELECT "+name+"(hashtextextended($1,0))", l.key)
			acquired = err == nil
		}
		if err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			return false, err
		}
		if !acquired {
			return false, nil
		}
		h.locks = append(h.locks, l)
	}
	h.active.Store(true)
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-work.Done():
				return
			case <-tick.C:
				h.mu.Lock()
				if work.Err() != nil {
					h.mu.Unlock()
					return
				}
				c, stop := context.WithTimeout(work, time.Second)
				e := conn.PingContext(c)
				stop()
				if e != nil {
					h.invalidate()
				}
				h.mu.Unlock()
				if e != nil {
					return
				}
			}
		}
	}()
	defer func() { h.active.Store(false); cancel(); <-done }()
	return true, fn(context.WithValue(work, configLockKey{}, h))
}
