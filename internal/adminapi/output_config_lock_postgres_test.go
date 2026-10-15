package adminapi

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func TestOutputConfigTryLockOwnershipAndCleanup(t *testing.T) {
	db := adminTestDB(t)
	db.SetMaxOpenConns(6)
	ctx := context.Background()
	blocker, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	defer blocker.ExecContext(ctx, "SELECT pg_advisory_unlock_all()")
	panel := "output-try-lock"
	key := "panel-config:" + panel
	if _, err = blocker.ExecContext(ctx, "SELECT pg_advisory_lock(hashtextextended($1,0))", key); err != nil {
		t.Fatal(err)
	}
	called := false
	start := time.Now()
	acquired, err := sanaei.TryWithConfigLock(ctx, db, panel, func(context.Context) error { called = true; return nil })
	if acquired || err != nil || called || time.Since(start) > time.Second {
		t.Fatalf("busy result=%v err=%v called=%v", acquired, err, called)
	}
	// Existing mutation callers still wait for ownership rather than skip it.
	waiting, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	err = sanaei.WithConfigLock(waiting, db, panel, func(context.Context) error { called = true; return nil })
	cancel()
	if err == nil || called {
		t.Fatal("blocking helper bypassed a held lock")
	}
	if _, err = blocker.ExecContext(ctx, "SELECT pg_advisory_unlock_all()"); err != nil {
		t.Fatal(err)
	}
	marker := errors.New("callback error")
	for _, mode := range []string{"normal", "error", "cancel", "panic"} {
		t.Run(mode, func(t *testing.T) {
			work, cancel := context.WithCancel(ctx)
			defer cancel()
			var captured context.Context
			var callbackErr error
			panicked := false
			func() {
				defer func() {
					if v := recover(); v != nil {
						if v != "expected" {
							panic(v)
						}
						panicked = true
					}
				}()
				acquired, callbackErr = sanaei.TryWithConfigLock(work, db, panel, func(owned context.Context) error {
					captured = owned
					if !sanaei.ConfigLockHeld(owned, db, panel) || sanaei.ConfigLockHeld(owned, db, "other") || sanaei.ConfigLockHeld(owned, new(sql.DB), panel) {
						t.Error("incorrect ownership scope")
					}
					var concurrent bool
					if e := blocker.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", key).Scan(&concurrent); e != nil || concurrent {
						t.Errorf("other session acquired owned lock: %v %v", concurrent, e)
					}
					switch mode {
					case "error":
						return marker
					case "cancel":
						cancel()
					case "panic":
						panic("expected")
					}
					return nil
				})
			}()
			if captured == nil || sanaei.ConfigLockHeld(captured, db, panel) {
				t.Fatal("ownership leaked beyond callback")
			}
			if mode == "panic" {
				if !panicked {
					t.Fatal("panic swallowed")
				}
			} else if !acquired {
				t.Fatal("free lock was not acquired")
			}
			if mode == "error" && !errors.Is(callbackErr, marker) {
				t.Fatal("callback error lost", callbackErr)
			}
			var free bool
			if e := blocker.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", key).Scan(&free); e != nil || !free {
				t.Fatalf("cleanup leaked lock: %v %v", free, e)
			}
			if _, e := blocker.ExecContext(ctx, "SELECT pg_advisory_unlock_all()"); e != nil {
				t.Fatal(e)
			}
		})
	}
}
