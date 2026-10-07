package worker

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAdmissionReservesNestedSQLAndSupervisionHeadroom(t *testing.T) {
	for _, tt := range []struct {
		role        Role
		slots, held int
	}{{RoleControl, 3, 5}, {RolePanels, 10, 2}} {
		t.Run(string(tt.role), func(t *testing.T) {
			db := failureLedgerFixture(t)
			db.SetMaxOpenConns(tt.role.ConnectionBudget())
			if _, err := db.Exec("CREATE TABLE worker_heartbeats(worker_id text PRIMARY KEY,kind text,last_seen_at timestamptz,metadata jsonb)"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			owner, err := AcquireRole(ctx, db, tt.role)
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Close()
			budget := NewWorkBudget(tt.slots)
			held := make(chan struct{}, tt.slots)
			query := make(chan struct{})
			release := make(chan struct{})
			var wg sync.WaitGroup
			results := make(chan error, tt.slots+5)
			for i := 0; i < tt.slots+5; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					results <- budget.Do(ctx, func(ctx context.Context) error {
						var conns []*sql.Conn
						defer func() {
							for _, c := range conns {
								c.Close()
							}
						}()
						for k := 0; k < tt.held; k++ {
							c, e := db.Conn(ctx)
							if e != nil {
								return e
							}
							conns = append(conns, c)
						}
						held <- struct{}{}
						select {
						case <-query:
						case <-ctx.Done():
							return ctx.Err()
						}
						// Every admitted holder needs MORE SQL to finish; waiting jobs must
						// acquire no connections until admission has succeeded.
						if _, e := db.ExecContext(ctx, "SELECT 1"); e != nil {
							return e
						}
						<-release
						return nil
					})
				}()
			}
			for i := 0; i < tt.slots; i++ {
				select {
				case <-held:
				case <-ctx.Done():
					t.Fatal("admitted holders stalled")
				}
			}
			if db.Stats().InUse != tt.slots*tt.held+1 {
				t.Fatal("unadmitted work borrowed SQL", db.Stats())
			}
			if e := owner.Check(ctx); e != nil {
				t.Fatal(e)
			}
			if e := (Heartbeat{DB: db, WorkerID: "headroom", Kind: tt.role.HeartbeatKind()}).Beat(ctx, map[string]any{"state": "fixture"}); e != nil {
				t.Fatal(e)
			}
			if due, e := (FailureStore{DB: db}).DueChecked(ctx, "lifecycle", "fixture"); e != nil || !due {
				t.Fatal("reconciliation blocked", e)
			}
			close(query)
			close(release)
			// Drain start notifications for jobs which enter after released holders.
			go func() {
				for range held {
				}
			}()
			wg.Wait()
			close(results)
			close(held)
			for e := range results {
				if e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}
func TestAdmissionCancellationDoesNotAbandonOrLeakSlots(t *testing.T) {
	b := NewWorkBudget(1)
	started := make(chan struct{})
	finish := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Do(ctx, func(context.Context) error { close(started); <-finish; return nil }) }()
	<-started
	cancel()
	c, stop := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer stop()
	if err := b.Do(c, func(context.Context) error { t.Error("overlap"); return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	close(finish)
	<-done
	if err := b.Do(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
}
