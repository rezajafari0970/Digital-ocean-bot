package scheduler

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"
)

const scheduleAccount = "00000000-0000-4000-8000-000000000021"
const scheduleProfile = "00000000-0000-4000-8000-000000000022"
const scheduleID = "00000000-0000-4000-8000-000000000023"

var injectedScheduleFailure = errors.New("injected isolated scheduler SQL failure")

type scheduleFaultConnector struct {
	driver.Connector
	query func(string) bool
	exec  func(string) bool
	rows  bool
}

func (c scheduleFaultConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &scheduleFaultConn{Conn: conn, query: c.query, exec: c.exec, rows: c.rows}, nil
}

type scheduleFaultConn struct {
	driver.Conn
	query func(string) bool
	exec  func(string) bool
	rows  bool
}

func (c *scheduleFaultConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}
func (c *scheduleFaultConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if c.query != nil && c.query(q) {
		return nil, injectedScheduleFailure
	}
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
	if err == nil && c.rows && strings.Contains(q, "FROM schedules") && strings.Contains(q, "FOR UPDATE SKIP LOCKED") {
		return &interruptedScheduleRows{Rows: rows}, nil
	}
	return rows, err
}
func (c *scheduleFaultConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	if c.exec != nil && c.exec(q) {
		return nil, injectedScheduleFailure
	}
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, q, args)
}

type interruptedScheduleRows struct {
	driver.Rows
	delivered bool
}

func (r *interruptedScheduleRows) Next(values []driver.Value) error {
	if r.delivered {
		return injectedScheduleFailure
	}
	err := r.Rows.Next(values)
	if err == nil {
		r.delivered = true
	}
	return err
}

// Each test uses its own schema in the mandatory gate's disposable database.
// No supplied production configuration or provider transport is used.
func schedulerFixture(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "bulk_test") {
		t.Fatal("disposable bulk_test database required")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("scheduler_durability_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(4)
	t.Cleanup(func() { db.Close(); admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	_, err = db.Exec(`
CREATE TABLE accounts(id uuid PRIMARY KEY,enabled boolean DEFAULT true,runtime_status text DEFAULT 'READY',provider_state text DEFAULT 'ACTIVE',provider_error_state text,provider text DEFAULT 'vultr',desired_server_count int DEFAULT 1,build_spacing_minutes int DEFAULT 60,build_spacing_max_minutes int DEFAULT 60,next_build_at timestamptz);
CREATE TABLE deployment_profiles(id uuid PRIMARY KEY,account_id uuid,name text,enabled boolean DEFAULT true,config jsonb,updated_at timestamptz);
CREATE TABLE installers(name text,version int,active boolean,manifest jsonb);
CREATE TABLE xui_database_templates(id uuid,active boolean,version int,name text);
CREATE TABLE deployments(id uuid,account_id uuid,profile_id uuid,state text,droplet_id uuid);
CREATE TABLE droplets(id uuid,account_id uuid,state text,backfill_required boolean,replacement_deployment_id uuid,updated_at timestamptz);
CREATE TABLE account_create_blocks(account_id uuid,version bigint,code text,blocked_at timestamptz);
CREATE TABLE provider_snapshots(account_id uuid,provider text,canonical jsonb,data jsonb,created_at timestamptz);
CREATE TABLE operations(account_id uuid,kind text,resource_id text,state text);
CREATE TABLE provider_capacity_observations(account_id uuid,source text,probe_in_flight boolean,probe_after timestamptz,updated_at timestamptz);
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000019_schedules.up.sql", "000020_concurrency_hardening.up.sql"} {
		raw, e := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if e != nil {
			t.Fatal(e)
		}
		// Migration 20 also changes unrelated deployment/operation tables.
		if strings.HasPrefix(name, "000020") {
			raw = []byte("ALTER TABLE schedules ADD COLUMN lease_until timestamptz;")
		}
		if _, e = db.Exec(string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	if raw, e := os.ReadFile(filepath.Join("..", "..", "migrations", "000196_scheduler_claim_fencing.up.sql")); e == nil {
		if _, e = db.Exec(string(raw)); e != nil {
			t.Fatal(e)
		}
	} else if !os.IsNotExist(e) {
		t.Fatal(e)
	}
	_, err = db.Exec(`
INSERT INTO accounts(id) VALUES('` + scheduleAccount + `');
INSERT INTO deployment_profiles(id,account_id,name,config) VALUES('` + scheduleProfile + `','` + scheduleAccount + `','scheduler-fixture','{"installer_ref":{"name":"fixture","version":1},"database_template_id":"00000000-0000-4000-8000-000000000024"}');
INSERT INTO installers VALUES('fixture',1,true,'{"capabilities":["xui_database","xui_panel"]}');
INSERT INTO xui_database_templates VALUES('00000000-0000-4000-8000-000000000024',true,1,'fixture');
INSERT INTO provider_snapshots VALUES('` + scheduleAccount + `','vultr','{"Capacity":{"ComputeLimit":3,"LimitKnown":true,"ComputeInUse":0},"Account":{"Status":"active"}}','{}',now());
INSERT INTO schedules(id,account_id,profile_id,interval_seconds,next_run_at) VALUES('` + scheduleID + `','` + scheduleAccount + `','` + scheduleProfile + `',60,now()-interval '1 hour');
`)
	if err != nil {
		t.Fatal(err)
	}
	return db, u.String()
}
func schedulerFaultDB(t *testing.T, dsn string, fault scheduleFaultConnector) *sql.DB {
	t.Helper()
	c, err := pq.NewConnector(dsn)
	if err != nil {
		t.Fatal(err)
	}
	fault.Connector = c
	db := sql.OpenDB(fault)
	db.SetMaxOpenConns(4)
	t.Cleanup(func() { db.Close() })
	return db
}

type schedulerStarter struct {
	prepare    func(context.Context, string) error
	prepareErr error
	starts     atomic.Int32
}

func (s *schedulerStarter) PrepareScheduledAccount(ctx context.Context, account string) error {
	if s.prepare != nil {
		return s.prepare(ctx, account)
	}
	return s.prepareErr
}
func (s *schedulerStarter) StartScheduledDeployment(context.Context, string, string, bool) error {
	s.starts.Add(1)
	return nil
}

func TestSchedulerClaimCompletionOwnershipPostgres(t *testing.T) {
	db, _ := schedulerFixture(t)
	ctx := context.Background()
	store := LeaseStore{DB: db}
	first, err := store.ClaimDue(ctx, time.Now().UTC(), 1, time.Minute)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim: %v %v", first, err)
	}
	if _, err = db.Exec("UPDATE schedules SET lease_until=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	second, err := store.ClaimDue(ctx, time.Now().UTC(), 1, time.Minute)
	if err != nil || len(second) != 1 {
		t.Fatalf("replacement claim: %v %v", second, err)
	}
	if err = store.Complete(ctx, first[0], time.Now().UTC()); err == nil {
		t.Error("stale completion cleared replacement ownership")
	}
	if err = (SQLStore{DB: db}).Advance(ctx, first[0], time.Now().UTC()); err == nil {
		t.Error("legacy advance bypassed claim ownership")
	}
	var leased bool
	if err = db.QueryRow("SELECT lease_until IS NOT NULL AND last_run_at IS NULL FROM schedules").Scan(&leased); err != nil || !leased {
		t.Fatalf("replacement altered: %v %v", leased, err)
	}
	wrong := second[0]
	wrong.AccountID = "00000000-0000-4000-8000-000000000099"
	if err = store.Complete(ctx, wrong, time.Now().UTC()); err == nil {
		t.Error("foreign account completed schedule")
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err = store.Complete(ctx, second[0], now); err != nil {
		t.Fatal(err)
	}
	var last, next time.Time
	if err = db.QueryRow("SELECT last_run_at,next_run_at FROM schedules").Scan(&last, &next); err != nil {
		t.Fatal(err)
	}
	if !last.Equal(now) || !next.Equal(now.Add(second[0].Interval)) {
		t.Fatal("completion cadence changed")
	}
	if err = store.Complete(ctx, second[0], now); err == nil {
		t.Error("completed claim was reusable")
	}
}

func TestSchedulerPartialClaimReadRollsBackPostgres(t *testing.T) {
	db, dsn := schedulerFixture(t)
	faultDB := schedulerFaultDB(t, dsn, scheduleFaultConnector{rows: true})
	items, err := (LeaseStore{DB: faultDB}).ClaimDue(context.Background(), time.Now().UTC(), 100, time.Minute)
	if !errors.Is(err, injectedScheduleFailure) || len(items) != 0 {
		t.Errorf("partial result accepted: items=%d err=%v", len(items), err)
	}
	var leased bool
	if err = db.QueryRow("SELECT lease_until IS NOT NULL FROM schedules").Scan(&leased); err != nil || leased {
		t.Fatalf("failed discovery committed lease: %v %v", leased, err)
	}
}

func TestSchedulerReadAndCompletionFaultsFailClosedPostgres(t *testing.T) {
	for _, which := range []string{"healthy", "prepare", "pending-capacity", "completion"} {
		t.Run(which, func(t *testing.T) {
			db, dsn := schedulerFixture(t)
			starter := &schedulerStarter{}
			if which == "prepare" {
				starter.prepareErr = injectedScheduleFailure
			}
			fault := scheduleFaultConnector{}
			if which == "pending-capacity" {
				fault.query = func(q string) bool {
					return strings.HasPrefix(strings.TrimSpace(q), "SELECT count(*) FROM deployments WHERE")
				}
			}
			if which == "completion" {
				fault.exec = func(q string) bool {
					return strings.HasPrefix(strings.TrimSpace(q), "UPDATE schedules SET last_run_at")
				}
			}
			if which == "pending-capacity" || which == "completion" {
				db = schedulerFaultDB(t, dsn, fault)
			}
			err := (Engine{DB: db, Leases: LeaseStore{DB: db}, Starter: starter}).RunDue(context.Background(), time.Now().UTC())
			if which == "healthy" {
				if err != nil || starter.starts.Load() != 1 {
					t.Fatalf("healthy scheduling failed: %v starts=%d", err, starter.starts.Load())
				}
				return
			}
			if err == nil {
				t.Error("failed round reported successful progress")
			}
			if (which == "prepare" || which == "pending-capacity") && starter.starts.Load() != 0 {
				t.Error("failed prerequisite dispatched creation")
			}
			if which == "completion" && starter.starts.Load() != 1 {
				t.Error("completion fault altered prior dispatch")
			}
		})
	}
}

// Ensure the injected row wrapper follows the normal driver end contract.
var _ driver.Rows = (*interruptedScheduleRows)(nil)

func TestSchedulerCancellationReplacementAndMigrationPostgres(t *testing.T) {
	t.Run("concurrent-claim", func(t *testing.T) {
		db, _ := schedulerFixture(t)
		store := LeaseStore{DB: db}
		results := make(chan []Schedule, 2)
		failures := make(chan error, 2)
		start := make(chan struct{})
		for range 2 {
			go func() {
				<-start
				items, err := store.ClaimDue(context.Background(), time.Now().UTC(), 1, time.Minute)
				results <- items
				failures <- err
			}()
		}
		close(start)
		count := 0
		for range 2 {
			count += len(<-results)
			if err := <-failures; err != nil {
				t.Error(err)
			}
		}
		if count != 1 {
			t.Fatalf("concurrent owners=%d", count)
		}
	})
	t.Run("foreign-profile", func(t *testing.T) {
		db, _ := schedulerFixture(t)
		if _, err := db.Exec("UPDATE deployment_profiles SET account_id='00000000-0000-4000-8000-000000000099'"); err != nil {
			t.Fatal(err)
		}
		items, err := (LeaseStore{DB: db}).ClaimDue(context.Background(), time.Now().UTC(), 1, time.Minute)
		if err != nil || len(items) != 0 {
			t.Fatalf("foreign profile claimed: %d %v", len(items), err)
		}
	})
	for _, mode := range []string{"cancelled", "replaced", "expired", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			db, _ := schedulerFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			starter := &schedulerStarter{prepare: func(context.Context, string) error {
				if mode == "cancelled" {
					cancel()
					return nil
				}
				query := "UPDATE schedules SET lease_token=gen_random_uuid()"
				if mode == "expired" {
					query = "UPDATE schedules SET lease_until=now()-interval '1 second'"
				}
				if mode == "disabled" {
					query = "UPDATE schedules SET enabled=false"
				}
				_, err := db.Exec(query)
				return err
			}}
			err := (Engine{DB: db, Leases: LeaseStore{DB: db}, Starter: starter}).RunDue(ctx, time.Now().UTC())
			if err == nil || starter.starts.Load() != 0 {
				t.Fatalf("invalid owner created: %v starts=%d", err, starter.starts.Load())
			}
			if mode == "replaced" {
				var retained bool
				if err = db.QueryRow("SELECT lease_token IS NOT NULL AND last_run_at IS NULL FROM schedules").Scan(&retained); err != nil || !retained {
					t.Fatalf("new claim lost: %v %v", retained, err)
				}
			}
			if mode == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation hidden: %v", err)
			}
		})
	}
	t.Run("downgrade-guard", func(t *testing.T) {
		db, _ := schedulerFixture(t)
		store := LeaseStore{DB: db}
		ctx := context.Background()
		items, err := store.ClaimDue(ctx, time.Now().UTC(), 1, time.Minute)
		if err != nil || len(items) != 1 {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000196_scheduler_claim_fencing.down.sql"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(raw)); err == nil {
			t.Fatal("downgrade removed retained claim")
		}
		if err = store.Validate(ctx, items[0]); err != nil {
			t.Fatal("failed downgrade changed live claim", err)
		}
		if err = store.Complete(ctx, items[0], time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(raw)); err != nil {
			t.Fatal("settled downgrade refused", err)
		}
	})
}
