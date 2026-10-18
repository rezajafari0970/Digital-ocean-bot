package main

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/residentialsync"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func proofTestDB(t *testing.T) *sql.DB { return proofTestDBThrough(t, 999999) }
func proofTestDBThrough(t *testing.T, maxVersion int) *sql.DB {
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
	id, _ := sanaei.UUIDv4()
	schema := "admin_" + strings.ReplaceAll(id, "-", "")
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
	db.SetMaxOpenConns(12)
	t.Cleanup(func() { db.Close(); admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// Historical migration 51 hard-codes public only for its catalog lookup.
	// Adapt that lookup to this isolated schema; production migration files stay unchanged.
	dir := t.TempDir()
	files, e := filepath.Glob("../../migrations/*.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	for _, file := range files {
		version, _ := strconv.Atoi(strings.Split(filepath.Base(file), "_")[0])
		if version > maxVersion {
			continue
		}
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		raw = []byte(strings.ReplaceAll(string(raw), "tc.table_schema='public'", "tc.table_schema=current_schema()"))
		if e = os.WriteFile(filepath.Join(dir, filepath.Base(file)), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if err = (migrate.Runner{DB: db, Dir: dir}).Up(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}
func proofSQL(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func proofOtherPool(t *testing.T, db *sql.DB) *sql.DB {
	t.Helper()
	var schema string
	if e := db.QueryRow("SELECT current_schema()").Scan(&schema); e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(os.Getenv("BULK_TEST_DATABASE_URL"))
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	other, e := sql.Open("postgres", u.String())
	if e != nil {
		t.Fatal(e)
	}
	other.SetMaxOpenConns(24)
	t.Cleanup(func() { other.Close() })
	return other
}
func TestProofPoolFortyPhysicalPinsAndProgress(t *testing.T) {
	t.Setenv("DOB_CONFIG_OBSERVATION_PANELS", "all")
	db := proofTestDB(t)
	db.SetMaxOpenConns(40)
	db.SetMaxIdleConns(40)
	api := proofOtherPool(t, db)
	proofSQL(t, db, "CREATE TABLE proof_pool_receipts(panel text PRIMARY KEY,owner integer,n integer)")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	role, e := worker.AcquireRole(ctx, db, worker.RolePanels)
	if e != nil {
		t.Fatal(e)
	}
	defer role.Close()
	// The global client executor is an additional pin beyond its config owner.
	// At most one cleanup/client/transport handler owns this shared executor.
	global, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer global.Close()
	if _, e = global.ExecContext(ctx, "SELECT pg_advisory_lock(628341902731)"); e != nil {
		t.Fatal(e)
	}
	defer global.ExecContext(context.Background(), "SELECT pg_advisory_unlock(628341902731)")
	live, retired, _ := newProofWorkBudgets(worker.RolePanels, "18")
	parent, _, _ := newPanelWorkBudgets(worker.RolePanels, 18)
	slots := int(parent.Snapshot()["limit"])
	ready, work, hold := make(chan struct{}, 19+slots), make(chan struct{}), make(chan struct{})
	errs := make(chan error, 40)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	for i := 0; i < 19; i++ {
		i := i
		b := live
		if i == 18 {
			b = retired
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- b.Do(ctx, func(c context.Context) error {
				panel := fmt.Sprintf("proof-%d", i)
				return sanaei.WithConfigLock(c, db, panel, func(c context.Context) error {
					if e := sanaei.ConfigProofOnly(c, db, panel); e != nil {
						return e
					}
					var owner int
					if e := sanaei.WithConfigEvidenceTx(c, db, panel, func(tx *sql.Tx) error { return tx.QueryRowContext(c, "SELECT pg_backend_pid()").Scan(&owner) }); e != nil {
						return e
					}
					ready <- struct{}{}
					select {
					case <-work:
					case <-c.Done():
						return c.Err()
					}
					// The real owning-connection helper must publish without another pool pin.
					for range 3 {
						var one int
						if e := db.QueryRowContext(c, "SELECT 1 FROM pg_sleep(0.02)").Scan(&one); e != nil {
							return e
						}
						if e := sanaei.WithConfigEvidenceTx(c, db, panel, func(tx *sql.Tx) error {
							var actual int
							if e := tx.QueryRowContext(c, "SELECT pg_backend_pid()").Scan(&actual); e != nil {
								return e
							}
							if actual != owner {
								return fmt.Errorf("receipt changed backend")
							}
							_, e := tx.ExecContext(c, "INSERT INTO proof_pool_receipts VALUES($1,$2,1) ON CONFLICT(panel) DO UPDATE SET n=proof_pool_receipts.n+1", panel, owner)
							return e
						}); e != nil {
							return e
						}
					}
					select {
					case <-hold:
						return nil
					case <-c.Done():
						return c.Err()
					}
				})
			})
		}()
	}
	for i := 0; i < slots; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- parent.Do(ctx, func(c context.Context) error {
				// Profiles retain their own lease in addition to the panel fence.
				// Holding only one connection here missed the production deadlock.
				lease, e := db.Conn(c)
				if e != nil {
					return e
				}
				defer lease.Close()
				if _, e = lease.ExecContext(c, "SELECT pg_advisory_lock(hashtextextended($1,0))", fmt.Sprintf("profile-%d", i)); e != nil {
					return e
				}
				defer lease.ExecContext(context.Background(), "SELECT pg_advisory_unlock_all()")
				conn, e := db.Conn(c)
				if e != nil {
					return e
				}
				defer conn.Close()
				var pid int
				if e = conn.QueryRowContext(c, "SELECT pg_backend_pid()").Scan(&pid); e != nil {
					return e
				}
				ready <- struct{}{}
				select {
				case <-work:
				case <-c.Done():
					return c.Err()
				}
				// Owner-session receipts and separate bounded discovery SQL both progress.
				for range 3 {
					var one int
					if e = db.QueryRowContext(c, "SELECT 1 FROM pg_sleep(0.02)").Scan(&one); e != nil {
						return e
					}
					if _, e = conn.ExecContext(c, "INSERT INTO proof_pool_receipts VALUES($1,$2,1) ON CONFLICT(panel) DO UPDATE SET n=proof_pool_receipts.n+1", fmt.Sprintf("guardian-%d", i), pid); e != nil {
						return e
					}
				}
				select {
				case <-hold:
					return nil
				case <-c.Done():
					return c.Err()
				}
			})
		}()
	}
	for range 19 + slots {
		select {
		case <-ready:
		case e := <-errs:
			t.Fatal(e)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if db.Stats().InUse != 21+2*slots || db.Stats().MaxOpenConnections != 40 {
		t.Fatal("physical ledger", db.Stats())
	}
	// A writer queued through the separate API pool must not steal worker headroom.
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- sanaei.WithConfigLock(ctx, api, "proof-0", func(context.Context) error { return nil })
	}()
	for {
		var queued bool
		e := api.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted AND classid::bigint=((hashtextextended('panel-config:proof-0',0)>>32)&4294967295) AND objid::bigint=(hashtextextended('panel-config:proof-0',0)&4294967295))`).Scan(&queued)
		if e != nil {
			t.Fatal(e)
		}
		if queued {
			break
		}
		time.Sleep(time.Millisecond * 5)
	}
	got, e := sanaei.TryWithConfigReadLock(ctx, api, "proof-0", func(c context.Context) error {
		return sanaei.WithConfigEvidenceTx(c, api, "proof-0", func(tx *sql.Tx) error { _, e := tx.ExecContext(c, "SELECT 1"); return e })
	})
	if e != nil || !got {
		t.Fatal("output starved behind queued writer", got, e)
	}
	close(work)
	for {
		var count, total int
		e := db.QueryRowContext(ctx, "SELECT count(*),COALESCE(sum(n),0) FROM proof_pool_receipts").Scan(&count, &total)
		if e != nil {
			t.Fatal(e)
		}
		if count == 19+slots && total == 3*(19+slots) {
			break
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if e = role.Check(ctx); e != nil {
		t.Fatal("role heartbeat lost", e)
	}
	stats := db.Stats()
	t.Logf("long_pins=%d reserved_short=%d max_open=40 pool_waits=%d", 21+2*slots, 40-(21+2*slots), stats.WaitCount)
	close(hold)
	wg.Wait()
	for range 19 + slots {
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
	}
	if e := <-writerDone; e != nil {
		t.Fatal(e)
	}
	if db.Stats().InUse != 2 {
		t.Fatal("leaked handlers", db.Stats())
	}
}

func TestProofFleetRealPostgresRenewal(t *testing.T) {
	if os.Getenv("DOB_PROOF_FLEET_TEST") != "1" {
		t.Skip("set DOB_PROOF_FLEET_TEST=1 for three real-time renewal cycles")
	}
	t.Setenv("DOB_CONFIG_OBSERVATION_PANELS", "all")
	db := proofTestDB(t)
	db.SetMaxOpenConns(40)
	db.SetMaxIdleConns(40)
	proofSQL(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	ids := make([]string, 91)
	for i := range ids {
		id, _ := sanaei.UUIDv4()
		ids[i] = id
		proofSQL(t, db, "INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,$2,'digitalocean','fixture')", id, fmt.Sprint(i))
		state := "READY"
		if i == 90 {
			state = "PENDING_DELETION"
		}
		proofSQL(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state,expires_at) VALUES($1,$1,$3,$2,now()+interval '1 hour')", id, state, id)
		proofSQL(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$1,'fixture','{}')", id)
		proofSQL(t, db, "INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step) VALUES($1,$1,$1,$1,'PANEL_COMPLETE','done')", id)
		proofSQL(t, db, "INSERT INTO panel_instances(id,account_id,droplet_id,driver,base_url,auth_secret_ref) VALUES($1,$1,$1,'sanaei-3x-ui','http://fixture','fixture')", id)
	}
	live, retired, _ := newProofWorkBudgets(worker.RolePanels, "18")
	service := residentialsync.Service{DB: db}
	claims := worker.FailureStore{DB: db}
	ctx, cancel := context.WithTimeout(context.Background(), 175*time.Second)
	defer cancel()
	type observation struct {
		last time.Time
		n    int
		gap  time.Duration
	}
	observations := map[string]observation{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	makeLane := func(serving bool, b *worker.WorkBudget) proofLane {
		return proofLane{admit: b.Do,
			discover: func(c context.Context, after string) (readyworker.Panel, bool, error) {
				return service.NextDuePanelForWorker(c, serving, after)
			},
			reserve: func(c context.Context, p readyworker.Panel) (worker.OutcomeReservation, bool, error) {
				return claims.ReserveOwnedOutcome(c, "residential_sync", p.ID, "", 90*time.Second)
			},
			reconcile: func(c context.Context, p readyworker.Panel) error {
				return sanaei.WithConfigLock(c, db, p.ID, func(c context.Context) error {
					if e := sanaei.ConfigProofOnly(c, db, p.ID); e != nil {
						return e
					}
					duration := 10 * time.Second
					if !serving {
						duration = 30 * time.Second
					}
					select {
					case <-time.After(duration):
					case <-c.Done():
						return c.Err()
					}
					var observed time.Time
					e := sanaei.WithConfigEvidenceTx(c, db, p.ID, func(tx *sql.Tx) error {
						return tx.QueryRowContext(c, `INSERT INTO panel_routing_state(panel_id,revision,state,verified_at,next_check_at)
 SELECT $1,revision,'APPLIED',clock_timestamp(),clock_timestamp()+interval '20 seconds' FROM residential_routing_control WHERE singleton
 ON CONFLICT(panel_id) DO UPDATE SET state='APPLIED',verified_at=EXCLUDED.verified_at,next_check_at=EXCLUDED.next_check_at RETURNING verified_at`, p.ID).Scan(&observed)
					})
					if e == nil {
						mu.Lock()
						v := observations[p.ID]
						if !v.last.IsZero() && observed.Sub(v.last) > v.gap {
							v.gap = observed.Sub(v.last)
						}
						v.last = observed
						v.n++
						observations[p.ID] = v
						mu.Unlock()
					}
					return e
				})
			},
			complete: func(c context.Context, p readyworker.Panel, l worker.OutcomeReservation, e error) error {
				return claims.RecordOwnedOutcome(c, "residential_sync", p.ID, "", l, e)
			},
		}
	}
	for i := 0; i < 19; i++ {
		serving := i < 18
		b := live
		if !serving {
			b = retired
		}
		l := makeLane(serving, b)
		wg.Add(1)
		go func() { defer wg.Done(); l.run(ctx) }()
	}
	done := false
	for ctx.Err() == nil {
		time.Sleep(time.Second)
		mu.Lock()
		complete := true
		for _, id := range ids[:90] {
			if observations[id].n < 3 {
				complete = false
				break
			}
		}
		mu.Unlock()
		if complete {
			done = true
			break
		}
	}
	cancel()
	wg.Wait()
	if !done {
		t.Fatal("fleet did not complete three renewal cycles")
	}
	var maximum time.Duration
	for _, id := range ids[:90] {
		v := observations[id]
		if v.gap > maximum {
			maximum = v.gap
		}
		if v.n < 3 || v.gap > 55*time.Second {
			t.Errorf("panel=%s receipts=%d max_gap=%s", id, v.n, v.gap)
		}
	}
	t.Logf("serving_panels=90 retired=1 serving_limit=18 worker_pool=40 max_receipt_gap=%s", maximum)
	if db.Stats().InUse != 0 {
		t.Fatal("fleet leaked SQL", db.Stats())
	}
}
