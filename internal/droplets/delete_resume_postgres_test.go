package droplets

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
)

func deleteResumeDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "bulk_test") {
		t.Fatal("isolated bulk_test database required")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	var suffix string
	if err = admin.QueryRow("SELECT replace(gen_random_uuid()::text,'-','')").Scan(&suffix); err != nil {
		t.Fatal(err)
	}
	schema := "delete_resume_" + suffix
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(16)
	t.Cleanup(func() { db.Close(); admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	dir := t.TempDir()
	files, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		raw, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		raw = []byte(strings.ReplaceAll(string(raw), "tc.table_schema='public'", "tc.table_schema=current_schema()"))
		if e = os.WriteFile(filepath.Join(dir, filepath.Base(f)), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = (migrate.Runner{DB: db, Dir: dir}).Up(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

type deleteReserveBarrier struct {
	jobs.Store
	arrived sync.WaitGroup
}

func (s *deleteReserveBarrier) Reserve(ctx context.Context, op jobs.Operation) (jobs.Operation, bool, error) {
	got, fresh, err := s.Store.Reserve(ctx, op)
	s.arrived.Done()
	s.arrived.Wait()
	return got, fresh, err
}

type deleteAtomicProvider struct {
	computeStub
	calls atomic.Int32
	check func() error
}

func (p *deleteAtomicProvider) DeleteServer(context.Context, string) error {
	p.calls.Add(1)
	if p.check != nil {
		return p.check()
	}
	return nil
}

func TestDeleteResumeConcurrentPostgres(t *testing.T) {
	db := deleteResumeDB(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		state jobs.OperationState
		empty bool
	}{
		{"planned-persisted", jobs.OperationPlanned, false},
		{"planned-reserve-crash", jobs.OperationPlanned, true},
		{"unknown", jobs.OperationUnknown, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var account string
			if err := db.QueryRow("INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'vultr','resume-fixture','fixture') RETURNING id::text").Scan(&account); err != nil {
				t.Fatal(err)
			}
			op := BuildDeleteOperation(account, "fixture-server-"+tc.name)
			store := jobs.SQLStore{DB: db}
			stored, _, err := store.Reserve(ctx, op)
			if err != nil {
				t.Fatal(err)
			}
			stored.State = tc.state
			if tc.empty {
				stored.ResourceID = ""
			}
			if err = store.Update(ctx, &stored); err != nil {
				t.Fatal(err)
			}
			const n = 12
			barrier := &deleteReserveBarrier{Store: store}
			barrier.arrived.Add(n)
			p := &deleteAtomicProvider{check: func() error {
				current, e := store.Get(ctx, op.AccountID, op.IdempotencyKey)
				if e != nil {
					return e
				}
				if current.State != jobs.OperationRunning || current.ResourceID != op.ResourceID {
					return errors.New("provider called before durable running claim")
				}
				return nil
			}}
			e := Executor{Operations: barrier, Provider: p, Gate: gateStub{}}
			errs := make(chan error, n)
			for i := 0; i < n; i++ {
				go func() { _, err := e.Delete(ctx, op, op.ResourceID); errs <- err }()
			}
			success := 0
			for i := 0; i < n; i++ {
				err := <-errs
				if err == nil {
					success++
				} else if !errors.Is(err, jobs.ErrOperationVersionConflict) {
					t.Fatal(err)
				}
			}
			got, err := store.Get(ctx, account, op.IdempotencyKey)
			if err != nil || success != 1 || p.calls.Load() != 1 || got.State != jobs.OperationVerifying || got.Attempt != 1 {
				t.Fatalf("success=%d calls=%d state=%s attempts=%d err=%v", success, p.calls.Load(), got.State, got.Attempt, err)
			}
			// Re-enter via the actual lifecycle engine: verifying advances locally,
			// but does not issue a second DELETE or claim provider absence.
			var droplet string
			if err = db.QueryRow("INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES(gen_random_uuid(),$1,$2,'RETIRING') RETURNING id::text", account, op.ResourceID).Scan(&droplet); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec("UPDATE operations SET idempotency_key=$2 WHERE id=$1", stored.ID, "lifecycle-delete:"+droplet); err != nil {
				t.Fatal(err)
			}
			engine := LifecycleEngine{Store: LifecycleStore{DB: db}, Executor: Executor{Operations: store, Provider: p, Gate: gateStub{}}}
			if err = engine.Process(ctx, LifecycleItem{ID: droplet, AccountID: account, ProviderID: op.ResourceID, State: Retiring}); err != nil {
				t.Fatal(err)
			}
			var state string
			if err = db.QueryRow("SELECT state FROM droplets WHERE id=$1", droplet).Scan(&state); err != nil {
				t.Fatal(err)
			}
			if state != "DELETING" || p.calls.Load() != 1 {
				t.Fatalf("lifecycle state=%s calls=%d", state, p.calls.Load())
			}
		})
	}
}
