package app

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"sync/atomic"
	"testing"
	"time"
)

type rollingProviderFactory struct {
	name    string
	observe func(context.Context, string) (providers.Observation, error)
}

func (f rollingProviderFactory) Name() string                 { return f.name }
func (f rollingProviderFactory) Metadata() providers.Metadata { return providers.Metadata{} }
func (f rollingProviderFactory) Open(_ context.Context, r providers.OpenRequest) (providers.Driver, error) {
	return rollingProviderDriver{f: f, id: r.AccountID}, nil
}

type rollingProviderDriver struct {
	providers.Driver
	f  rollingProviderFactory
	id string
}

func (d rollingProviderDriver) Observe(ctx context.Context) (providers.Observation, error) {
	return d.f.observe(ctx, d.id)
}
func (d rollingProviderDriver) ObserveFast(ctx context.Context) (providers.Observation, error) {
	return d.Observe(ctx)
}
func runProviderCoordinatorFixture(ctx context.Context, c Container) {
	c.RunProviderSnapshots(ctx, nil)
}

func TestProviderRollingHealthyAccountPublishesBeforeSlowPeer(t *testing.T) {
	db := installerBootstrapDB(t)
	a, b := observationFixture(t, db), observationFixture(t, db)
	registry := providers.NewRegistry()
	first := make(chan string, 1)
	next := make(chan string, 1)
	var calls atomic.Int32
	registry.Register(rollingProviderFactory{name: "digitalocean", observe: func(ctx context.Context, id string) (providers.Observation, error) {
		if calls.Add(1) == 1 {
			first <- id
			<-ctx.Done()
			return providers.Observation{}, ctx.Err()
		}
		next <- id
		return providers.Observation{Account: providers.Account{ID: id, Status: "active"}, Capacity: providers.Capacity{LimitKnown: true, ComputeLimit: 10}, ObservedAt: time.Now()}, nil
	}})
	c := Container{DB: db, Accounts: Repository{DB: db}, Providers: registry}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); runProviderCoordinatorFixture(ctx, c) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("coordinator did not join")
		}
	}()
	select {
	case <-first:
	case <-time.After(3 * time.Second):
		t.Fatal("first observation not started")
	}
	var healthy string
	select {
	case healthy = <-next:
	case <-time.After(3 * time.Second):
		t.Fatal("healthy account blocked behind slow peer")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if e := db.QueryRow("SELECT count(*) FROM provider_snapshots WHERE account_id=$1 AND canonical IS NOT NULL", healthy).Scan(&count); e != nil {
			t.Fatal(e)
		}
		if count == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("healthy observation was not published: %s %s", a.Config.ID, b.Config.ID)
}

func TestProviderRollingTwoTimeoutsDoNotBlockHealthyTail(t *testing.T) {
	db := installerBootstrapDB(t)
	for range 10 {
		observationFixture(t, db)
	}
	var calls atomic.Int32
	r := providers.NewRegistry()
	r.Register(rollingProviderFactory{name: "digitalocean", observe: func(ctx context.Context, id string) (providers.Observation, error) {
		if calls.Add(1) <= 2 {
			<-ctx.Done()
			return providers.Observation{}, ctx.Err()
		}
		return providers.Observation{Account: providers.Account{ID: id, Status: "active"}, Capacity: providers.Capacity{LimitKnown: true, ComputeLimit: 10}, ObservedAt: time.Now()}, nil
	}})
	c := Container{DB: db, Accounts: Repository{DB: db}, Providers: r}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.runProviderSnapshots(ctx, nil, providerRefreshOptions{200 * time.Millisecond, 30 * time.Second, 5 * time.Millisecond})
	}()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var healthy, failed, forged int
		e := db.QueryRow("SELECT count(*) FILTER(WHERE provider_checked_at IS NOT NULL),count(*) FILTER(WHERE provider_error_state='TRANSPORT_ERROR'),count(*) FILTER(WHERE provider_error_state='TRANSPORT_ERROR' AND provider_checked_at IS NOT NULL) FROM accounts").Scan(&healthy, &failed, &forged)
		if e != nil {
			t.Fatal(e)
		}
		if forged != 0 {
			t.Fatal("local timeout forged provider contact")
		}
		if healthy == 8 && failed == 2 {
			if calls.Load() != 10 {
				t.Fatal("timeout retried without backoff", calls.Load())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("healthy tail did not progress after bounded timeouts", calls.Load())
}

func TestProviderRefreshFailureBackoffStartsAtFailurePublication(t *testing.T) {
	db := installerBootstrapDB(t)
	rt := observationFixture(t, db)
	c := Container{DB: db}
	execBootstrap(t, db, "UPDATE accounts SET provider_error_state='TRANSPORT_ERROR',provider_error_at=now(),provider_refresh_attempt_at=now()-interval '45 seconds' WHERE id=$1", rt.Config.ID)
	if _, due := c.providerRefreshDue(context.Background(), rt.Config.ID, time.Minute, 30*time.Second); due {
		t.Fatal("45-second timeout is immediately eligible for another attempt")
	}
	ids, e := c.discoverProviderRefreshes(context.Background(), nil, 2, 30*time.Second)
	if e != nil || len(ids) != 0 {
		t.Fatal("discovery bypassed failure backoff", ids, e)
	}
}

func TestProviderScheduledAttemptRejectsRetiredAccounts(t *testing.T) {
	for _, change := range []string{"enabled=false", "deleted_at=now()", "deletion_requested_at=now()"} {
		t.Run(change, func(t *testing.T) {
			db := installerBootstrapDB(t)
			rt := observationFixture(t, db)
			c := Container{DB: db}
			execBootstrap(t, db, "UPDATE accounts SET "+change+" WHERE id=$1", rt.Config.ID)
			if _, e := c.BeginProviderAttempt(context.Background(), rt.Config.ID, true); e == nil {
				t.Fatal("retired account started an attempt")
			}
			var attempted bool
			if e := db.QueryRow("SELECT provider_refresh_attempt_at IS NOT NULL FROM accounts WHERE id=$1", rt.Config.ID).Scan(&attempted); e != nil || attempted {
				t.Fatal(attempted, e)
			}
		})
	}
}

func TestProviderRollingCancellationJoinsExecutingHandler(t *testing.T) {
	db := installerBootstrapDB(t)
	observationFixture(t, db)
	entered := make(chan struct{})
	release := make(chan struct{})
	r := providers.NewRegistry()
	r.Register(rollingProviderFactory{name: "digitalocean", observe: func(ctx context.Context, id string) (providers.Observation, error) {
		close(entered)
		<-release
		return providers.Observation{}, ctx.Err()
	}})
	c := Container{DB: db, Accounts: Repository{DB: db}, Providers: r}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var active atomic.Int32
	admit := func(ctx context.Context, fn func(context.Context) error) error {
		active.Add(1)
		defer active.Add(-1)
		return fn(ctx)
	}
	go func() { defer close(done); c.RunProviderSnapshots(ctx, admit) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		cancel()
		close(release)
		t.Fatal("handler not started")
	}
	cancel()
	select {
	case <-done:
		close(release)
		t.Fatal("cancellation detached a live handler")
	case <-time.After(30 * time.Millisecond):
	}
	if active.Load() != 1 {
		close(release)
		t.Fatal("executing handler lost admission", active.Load())
	}
	close(release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not join")
	}
	if active.Load() != 0 {
		t.Fatal("admission leaked")
	}
	var failures int
	if e := db.QueryRow("SELECT count(*) FROM accounts WHERE provider_error_state IS NOT NULL").Scan(&failures); e != nil || failures != 0 {
		t.Fatal("shutdown invented provider outage", failures, e)
	}
}

func TestProviderRollingAdmissionPrecedesSQL(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	entered := make(chan struct{})
	admit := func(ctx context.Context, fn func(context.Context) error) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); c.RunProviderSnapshots(ctx, admit) }()
	<-entered
	if inUse := db.Stats().InUse; inUse != 0 {
		cancel()
		<-done
		t.Fatal("SQL held awaiting admission", inUse)
	}
	cancel()
	<-done
}

func TestProviderRefreshStaleCredentialsCannotWriteInventory(t *testing.T) {
	for _, provider := range []string{"vultr", "upcloud", "linode"} {
		t.Run(provider, func(t *testing.T) {
			db := installerBootstrapDB(t)
			rt := observationFixture(t, db)
			execBootstrap(t, db, "UPDATE accounts SET provider=$2 WHERE id=$1", rt.Config.ID, provider)
			execBootstrap(t, db, "INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed,metadata) VALUES(gen_random_uuid(),$1,$2,'retained','server','running',true,'{}')", rt.Config.ID, provider)
			registry := providers.NewRegistry()
			registry.Register(rollingProviderFactory{name: provider, observe: func(ctx context.Context, id string) (providers.Observation, error) {
				execBootstrap(t, db, "UPDATE accounts SET secret_ref='rotated' WHERE id=$1", id)
				return providers.Observation{Account: providers.Account{ID: id, Status: "active"}, Inventory: providers.Inventory{Servers: []providers.Server{}, ObservedAt: time.Now()}, Capacity: providers.Capacity{ComputeInUse: 4}, ObservedAt: time.Now()}, nil
			}})
			c := Container{DB: db, Accounts: Repository{DB: db}, Providers: registry}
			c.refreshProviderAccount(context.Background(), rt.Config.ID, time.Minute, 0, 45*time.Second)
			var state string
			var capacityWrites, snapshots int
			if e := db.QueryRow("SELECT state,(SELECT count(*) FROM provider_capacity_observations WHERE account_id=$1),(SELECT count(*) FROM provider_snapshots WHERE account_id=$1) FROM resources WHERE account_id=$1 AND provider_resource_id='retained'", rt.Config.ID).Scan(&state, &capacityWrites, &snapshots); e != nil {
				t.Fatal(e)
			}
			if state != "running" || capacityWrites != 0 || snapshots != 0 {
				t.Fatal("rejected credentials changed inventory/capacity", state, capacityWrites, snapshots)
			}
		})
	}
}

func (d rollingProviderDriver) Catalog(context.Context) (providers.Catalog, error) {
	return providers.Catalog{}, nil
}

func TestProviderRollingContendedHeadsYieldToOtherAccounts(t *testing.T) {
	db := installerBootstrapDB(t)
	a, b, cfg := observationFixture(t, db), observationFixture(t, db), observationFixture(t, db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var locks []*sql.Tx
	for _, id := range []string{a.Config.ID, b.Config.ID} {
		tx, e := db.BeginTx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		locks = append(locks, tx)
		defer tx.Rollback()
		if _, e = tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "provider-refresh:"+id); e != nil {
			t.Fatal(e)
		}
		execBootstrap(t, db, "UPDATE accounts SET provider_state_at=now()-interval '1 hour' WHERE id=$1", id)
	}
	called := make(chan string, 1)
	r := providers.NewRegistry()
	r.Register(rollingProviderFactory{name: "digitalocean", observe: func(ctx context.Context, id string) (providers.Observation, error) {
		called <- id
		return providers.Observation{Account: providers.Account{ID: id, Status: "active"}}, nil
	}})
	c := Container{DB: db, Accounts: Repository{DB: db}, Providers: r}
	done := make(chan struct{})
	go func() { defer close(done); c.RunProviderSnapshots(ctx, nil) }()
	defer func() { cancel(); <-done }()
	select {
	case id := <-called:
		if id != cfg.Config.ID {
			t.Fatal("contended account reached provider")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("locked heads starved healthy account")
	}
}

func TestProviderRollingCompetingCoordinatorsObserveOnce(t *testing.T) {
	db := installerBootstrapDB(t)
	observationFixture(t, db)
	var count atomic.Int32
	entered := make(chan struct{}, 2)
	r := providers.NewRegistry()
	r.Register(rollingProviderFactory{name: "digitalocean", observe: func(ctx context.Context, id string) (providers.Observation, error) {
		count.Add(1)
		entered <- struct{}{}
		<-ctx.Done()
		return providers.Observation{}, ctx.Err()
	}})
	c := Container{DB: db, Accounts: Repository{DB: db}, Providers: r}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{}, 2)
	for range 2 {
		go func() { c.RunProviderSnapshots(ctx, nil); done <- struct{}{} }()
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("no observation")
	}
	select {
	case <-entered:
		t.Fatal("same account observed concurrently")
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	<-done
	<-done
	if count.Load() != 1 {
		t.Fatal("duplicate observation", count.Load())
	}
}

func TestProviderRefreshDiscoveryMatchesCadenceAndBackoff(t *testing.T) {
	db := installerBootstrapDB(t)
	rt := observationFixture(t, db)
	c := Container{DB: db, Economy: &network.EconomyController{DB: db}}
	execBootstrap(t, db, "UPDATE proxy_economy_policy SET enabled=true,canary_account_id=NULL WHERE singleton")
	execBootstrap(t, db, "UPDATE accounts SET next_build_at=now()+interval '20 minutes',provider_inventory_interval_seconds=300,provider_checked_at=now()-interval '200 seconds' WHERE id=$1", rt.Config.ID)
	execBootstrap(t, db, "INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical,created_at) VALUES(gen_random_uuid(),$1,'digitalocean',2,'{}','{}',now()-interval '200 seconds')", rt.Config.ID)
	for _, scenario := range []string{"idle_fresh", "idle_due", "transition", "restriction"} {
		switch scenario {
		case "idle_due":
			execBootstrap(t, db, "UPDATE provider_snapshots SET created_at=now()-interval '280 seconds' WHERE account_id=$1", rt.Config.ID)
		case "transition":
			execBootstrap(t, db, "UPDATE accounts SET next_build_at=now()+interval '1 minute',provider_checked_at=now() WHERE id=$1", rt.Config.ID)
			execBootstrap(t, db, "UPDATE provider_snapshots SET created_at=now() WHERE account_id=$1", rt.Config.ID)
		case "restriction":
			execBootstrap(t, db, "UPDATE accounts SET provider_state='TOKEN_INVALID',provider_refresh_attempt_at=now()-interval '1 minute' WHERE id=$1", rt.Config.ID)
		}
		want := scenario == "idle_due" || scenario == "transition"
		_, due := c.providerRefreshDue(context.Background(), rt.Config.ID, time.Minute, 30*time.Second)
		ids, e := c.discoverProviderRefreshes(context.Background(), nil, 2, 30*time.Second)
		if e != nil || due != want || (len(ids) == 1) != want {
			t.Fatal(scenario, due, ids, e)
		}
	}
}

func TestProviderRollingProviderParity(t *testing.T) {
	for _, provider := range []string{"digitalocean", "vultr", "upcloud", "linode"} {
		for _, economy := range []bool{false, true} {
			for _, warm := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/economy=%v/warm=%v", provider, economy, warm), func(t *testing.T) {
					db := installerBootstrapDB(t)
					rt := observationFixture(t, db)
					execBootstrap(t, db, "UPDATE accounts SET provider=$2 WHERE id=$1", rt.Config.ID, provider)
					execBootstrap(t, db, "UPDATE proxy_economy_policy SET enabled=$1,canary_account_id=NULL WHERE singleton", economy)
					if warm {
						execBootstrap(t, db, "INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical,created_at) VALUES(gen_random_uuid(),$1,$2,2,'{}','{}',now()-interval '10 minutes')", rt.Config.ID, provider)
					}
					var observed atomic.Int32
					registry := providers.NewRegistry()
					registry.Register(rollingProviderFactory{name: provider, observe: func(ctx context.Context, id string) (providers.Observation, error) {
						if _, ok := ctx.Deadline(); !ok {
							t.Error("provider received unbounded context")
						}
						observed.Add(1)
						return providers.Observation{Account: providers.Account{ID: id, Status: "active"}, Inventory: providers.Inventory{Servers: []providers.Server{}, ObservedAt: time.Now()}, Capacity: providers.Capacity{LimitKnown: provider != "vultr", ComputeLimit: 10}, ObservedAt: time.Now()}, nil
					}})
					c := Container{DB: db, Accounts: Repository{DB: db}, Providers: registry, Economy: &network.EconomyController{DB: db}}
					c.refreshProviderAccount(context.Background(), rt.Config.ID, time.Minute, 30*time.Second, 45*time.Second)
					var fresh bool
					var interval int
					if e := db.QueryRow("SELECT provider_checked_at>now()-interval '5 seconds',provider_inventory_interval_seconds FROM accounts WHERE id=$1", rt.Config.ID).Scan(&fresh, &interval); e != nil || !fresh || interval != 60 || observed.Load() != 1 {
						t.Fatal(fresh, interval, observed.Load(), e)
					}
					if provider == "vultr" {
						var known bool
						if e := db.QueryRow("SELECT (canonical->'Capacity'->>'LimitKnown')::bool FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1", rt.Config.ID).Scan(&known); e != nil || known {
							t.Fatal("unknown Vultr capacity became known", known, e)
						}
					}
				})
			}
		}
	}
}

func TestProviderRefreshIdlePeerCannotHideActiveTransition(t *testing.T) {
	db := installerBootstrapDB(t)
	a, b := observationFixture(t, db), observationFixture(t, db)
	c := Container{DB: db, Economy: &network.EconomyController{DB: db}}
	execBootstrap(t, db, "UPDATE proxy_economy_policy SET enabled=true,canary_account_id=NULL WHERE singleton")
	for _, id := range []string{a.Config.ID, b.Config.ID} {
		execBootstrap(t, db, "UPDATE accounts SET provider_inventory_interval_seconds=300,provider_checked_at=now()-interval '40 seconds',desired_server_count=1,next_build_at=now()+interval '20 minutes' WHERE id=$1", id)
		execBootstrap(t, db, "INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical,created_at) VALUES(gen_random_uuid(),$1,'digitalocean',2,'{}','{}',now()-interval '40 seconds')", id)
	}
	execBootstrap(t, db, "UPDATE accounts SET next_build_at=now()+interval '1 minute' WHERE id=$1", b.Config.ID)
	age, due := c.providerRefreshDue(context.Background(), b.Config.ID, time.Minute, 30*time.Second)
	if !due || age != time.Minute {
		t.Fatal("fixture did not transition", age, due)
	}
	ids, e := c.discoverProviderRefreshes(context.Background(), nil, 2, 30*time.Second)
	if e != nil || len(ids) != 1 || ids[0] != b.Config.ID {
		t.Fatal("idle peer hid active transition", ids, e)
	}
}
