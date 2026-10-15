package adminapi

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"testing"
	"time"
)

func TestOutputRefreshLeavesDatabaseHeadroomWhileLocksBusy(t *testing.T) {
	for _, budget := range []int{6, 24} {
		t.Run(fmt.Sprint(budget), func(t *testing.T) {
			db := adminTestDB(t)
			db.SetMaxOpenConns(budget)
			ctx := context.Background()
			blocker, err := db.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			s := New(db, app.Container{})
			defer func() {
				_, _ = blocker.ExecContext(ctx, "SELECT pg_advisory_unlock_all()")
				blocker.Close()
				until := time.Now().Add(7 * time.Second)
				for time.Now().Before(until) {
					s.OutputPanelMu.Lock()
					n := len(s.OutputPanelRun)
					s.OutputPanelMu.Unlock()
					if n == 0 {
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
				t.Error("output refreshes failed to drain after releasing locks")
			}()
			panels := make([]readyworker.Panel, 64)
			for i := range panels {
				id, e := sanaei.UUIDv4()
				if e != nil {
					t.Fatal(e)
				}
				panels[i] = readyworker.Panel{ID: id}
				if _, e = blocker.ExecContext(ctx, "SELECT pg_advisory_lock(hashtextextended($1,0))", "panel-config:"+id); e != nil {
					t.Fatal(e)
				}
			}
			admitted := 0
			for _, p := range panels {
				if s.startPanelOutputRefresh(ctx, p) {
					admitted++
				}
			}
			if admitted == 0 {
				t.Fatal("fixture did not start output refreshes")
			}
			// Busy panel locks must promptly release scarce collector slots, even
			// while all blocker locks remain held.
			until := time.Now().Add(time.Second)
			drained := false
			for time.Now().Before(until) {
				s.OutputPanelMu.Lock()
				drained = len(s.OutputPanelRun) == 0 && len(s.OutputRefreshSem) == 0
				s.OutputPanelMu.Unlock()
				if drained {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if !drained {
				t.Fatal("busy locks kept output collectors and pool slots occupied")
			}
			probe, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
			defer cancel()
			if err := db.PingContext(probe); err != nil {
				t.Fatalf("output collectors starved readiness SQL: budget=%d admitted=%d: %v", budget, admitted, err)
			}
		})
	}
}
