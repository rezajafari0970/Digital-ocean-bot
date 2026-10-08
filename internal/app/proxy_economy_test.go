package app

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type economyRoundTrip func(*http.Request) (*http.Response, error)

func (f economyRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestEconomyIdentityFallback(t *testing.T) {
	for _, first := range []string{"ok", "invalid", "auth", "cancel"} {
		t.Run(first, func(t *testing.T) {
			calls := 0
			g := &network.Gateway{Client: &http.Client{Transport: economyRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					switch first {
					case "auth":
						return nil, errors.New("username/password authentication failed")
					case "cancel":
						<-r.Context().Done()
						return nil, r.Context().Err()
					case "invalid":
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ip":"::1"}`)), Header: make(http.Header)}, nil
					}
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ip":"203.0.113.9"}`)), Header: make(http.Header)}, nil
			})}}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			ip, err := fastGatewayExitIP(ctx, g)
			want := 1
			if first == "invalid" {
				want = 2
			}
			if calls != want {
				t.Fatalf("calls=%d want=%d", calls, want)
			}
			if first == "auth" || first == "cancel" {
				if err == nil {
					t.Fatal("failure admitted")
				}
			} else if err != nil || ip != "203.0.113.9" {
				t.Fatalf("ip=%s err=%v", ip, err)
			}
		})
	}
}

type economyDriver struct {
	providers.Driver
	catalogCalls atomic.Int32
	fastCalls    atomic.Int32
	fail         atomic.Bool
}

func (d *economyDriver) Catalog(context.Context) (providers.Catalog, error) {
	d.catalogCalls.Add(1)
	if d.fail.Load() {
		return providers.Catalog{}, errors.New("catalog unavailable")
	}
	return providers.Catalog{Plans: []providers.Plan{{ID: "p"}}}, nil
}
func (d *economyDriver) Observe(context.Context) (providers.Observation, error) {
	return providers.Observation{}, errors.New("full observation unexpectedly called")
}
func (d *economyDriver) ObserveFast(context.Context) (providers.Observation, error) {
	d.fastCalls.Add(1)
	return providers.Observation{Capacity: providers.Capacity{ComputeLimit: 7, LimitKnown: true, ObservedAt: time.Now()}}, nil
}
func TestEconomyCatalogCoalescingPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db.SetMaxOpenConns(4)
	var id string
	if err := db.QueryRow(`INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'digitalocean','economy','unused') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE proxy_economy_policy SET enabled=true`); err != nil {
		t.Fatal(err)
	}
	c := Container{DB: db, Economy: &network.EconomyController{DB: db}}
	d := &economyDriver{}
	rt := AccountRuntime{Config: AccountConfig{ID: id, Provider: "digitalocean", SecretRef: "unused"}, Driver: d}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			obs, err := c.ObserveProvider(ctx, rt)
			if err != nil || len(obs.Catalog.Plans) != 1 || obs.Capacity.ComputeLimit != 7 {
				t.Errorf("obs=%+v err=%v", obs, err)
			}
		}()
	}
	wg.Wait()
	if d.catalogCalls.Load() != 1 || d.fastCalls.Load() != 16 {
		t.Fatalf("catalog=%d fresh=%d", d.catalogCalls.Load(), d.fastCalls.Load())
	}
	if _, err := db.Exec(`UPDATE provider_catalog_cache SET refreshed_at=now()-interval '7 hours' WHERE account_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	d.fail.Store(true)
	if _, err := c.ObserveProvider(ctx, rt); err == nil {
		t.Fatal("expired failed catalog served")
	}
	d.fail.Store(false)
	if _, err := c.ObserveProvider(ctx, rt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO secrets(id,account_id,kind,ciphertext,nonce,key_version) VALUES('unused',$1,'token','a','b',1)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ObserveProvider(ctx, rt); err != nil {
		t.Fatal(err)
	}
	if d.catalogCalls.Load() != 4 {
		t.Fatalf("credential/expiry invalidation calls=%d", d.catalogCalls.Load())
	}
	if _, err := db.Exec(`UPDATE proxy_economy_policy SET enabled=false`); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ObserveProvider(ctx, rt); err == nil {
		t.Fatal("rollback did not select full observation")
	}
}
func TestEconomyAdmissionAndCanaryPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c := Container{DB: db, Economy: &network.EconomyController{DB: db}}
	var id, pid string
	if err := db.QueryRow(`INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'digitalocean','economy-fresh','unused') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO proxies(id,name,type,host,port,status,last_success_at,consecutive_successes) VALUES(gen_random_uuid(),'economy-proxy','socks5','127.0.0.1',9,'healthy',now(),2) RETURNING id::text`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,'proxy_required',$2)`, id, pid)
	exec(`INSERT INTO account_network_identities(account_id,exit_ip,subnet_key,last_health_ok,last_health_at,sticky_session) VALUES($1,'203.0.113.44','203.0.113.0/24',true,now(),'s')`, id)
	exec(`INSERT INTO proxy_runtime_state(account_id,proxy_id,provider,generation,health_state,circuit_state) VALUES($1,$2,'digitalocean',1,'healthy','closed')`, id, pid)
	exec(`INSERT INTO account_proxy_pool(account_id,proxy_id,priority,enabled) VALUES($1,$2,0,true)`, id, pid)
	exec(`UPDATE proxy_economy_policy SET enabled=true,canary_account_id=$1`, id)
	if ok, err := c.Economy.Enabled(ctx, ""); err != nil || ok {
		t.Fatal("canary slowed shared monitor", err)
	}
	if err := c.requireAccountNetworkReady(ctx, id); err != nil {
		t.Fatal(err)
	}
	var identityAt time.Time
	if err := db.QueryRow(`SELECT last_health_at FROM account_network_identities WHERE account_id=$1`, id).Scan(&identityAt); err != nil {
		t.Fatal(err)
	}
	var scheduled sync.WaitGroup
	for n := 0; n < 12; n++ {
		scheduled.Add(1)
		go func() {
			defer scheduled.Done()
			if err := (ScheduledStarter{Container: c}).PrepareScheduledAccount(ctx, id); err != nil {
				t.Errorf("fresh scheduled preparation must reuse proof, not dial closed fixture proxy: %v", err)
			}
		}()
	}
	scheduled.Wait()
	exec(`UPDATE accounts SET enabled=false WHERE id=$1`, id)
	if err := (ScheduledStarter{Container: c}).PrepareScheduledAccount(ctx, id); !errors.Is(err, ErrAccountDisabled) {
		t.Fatal("inactive account entered scheduled maintenance", err)
	}
	exec(`UPDATE accounts SET enabled=true WHERE id=$1`, id)
	var afterAt time.Time
	if err := db.QueryRow(`SELECT last_health_at FROM account_network_identities WHERE account_id=$1`, id).Scan(&afterAt); err != nil || !afterAt.Equal(identityAt) {
		t.Fatal("scheduled check forged a fresh observation", err)
	}

	if ok, err := c.identityProbeMayWait(ctx, id); err != nil || !ok {
		t.Fatal("fresh identity should wait", err)
	}
	exec(`UPDATE account_network_identities SET last_health_at=now()-interval '125 seconds' WHERE account_id=$1`, id)
	if ok, err := c.identityProbeMayWait(ctx, id); err != nil || ok {
		t.Fatal("probe overdue", err)
	}
	exec(`UPDATE account_network_identities SET last_health_at=now()-interval '181 seconds' WHERE account_id=$1`, id)
	if err := c.requireAccountNetworkReady(ctx, id); !errors.Is(err, ErrNetworkNotReady) {
		t.Fatal("stale admitted", err)
	}
	exec(`UPDATE account_network_identities SET last_health_at=now() WHERE account_id=$1`, id)
	exec(`UPDATE proxies SET last_success_at=now()-interval '361 seconds' WHERE id=$1`, pid)
	if err := c.requireAccountNetworkReady(ctx, id); !errors.Is(err, ErrNetworkNotReady) {
		t.Fatal("stale base admitted", err)
	}
	exec(`UPDATE proxy_economy_policy SET enabled=false`)
	if err := c.requireAccountNetworkReady(ctx, id); err != nil {
		t.Fatal("rollback", err)
	}
	exec(`UPDATE proxy_economy_policy SET enabled=true,canary_account_id=NULL`)
	exec(`UPDATE accounts SET next_build_at=now()+interval '1 hour' WHERE id=$1`, id)
	if age := c.providerRefreshAge(ctx, id, time.Minute); age != 5*time.Minute {
		t.Fatal("idle interval", age)
	}
	exec(`INSERT INTO droplets(id,account_id,provider_resource_id,state,expires_at) VALUES(gen_random_uuid(),$1,'fixture','READY',now()+interval '4 minutes')`, id)
	if age := c.providerRefreshAge(ctx, id, time.Minute); age != time.Minute {
		t.Fatal("expiry must be active", age)
	}
}

func TestEconomyExistingRouteGuardsPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	var schema string
	if err := db.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("BULK_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	t.Setenv("DOB_E2E_DSN", u.String())
	t.Run("stale-identity-generation-route", TestAccountNetworkGuardRejectsReassignmentBeforeNetworkPostgresE2E)
	t.Run("inflight-cutover", TestRouteCutoverWaitsForInflightSharedLockPostgresE2E)
	t.Run("keeper-serialization", TestProxyKeeperLockSerializesAcrossConnectionsPostgresE2E)
	t.Run("pool-epoch", TestProxyPoolSwitchBumpsEpochAtomicallyPostgresE2E)
}

func TestEconomyIdentitySlowPrimaryRetainsCallerBudget(t *testing.T) {
	var calls atomic.Int32
	g := &network.Gateway{Client: &http.Client{Transport: economyRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.Host == "api.ipify.org" {
			select {
			case <-time.After(1700 * time.Millisecond):
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ip":"203.0.113.9"}`)), Header: make(http.Header)}, nil
	})}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ip, err := fastGatewayExitIP(ctx, g)
	if err != nil || ip != "203.0.113.9" || calls.Load() != 1 {
		t.Fatalf("cold healthy primary incorrectly timed out: ip=%s err=%v calls=%d", ip, err, calls.Load())
	}
}
func TestEconomyOffRestoresParallelIdentity(t *testing.T) {
	started := make(chan string, 2)
	g := &network.Gateway{AccountID: "a", Client: &http.Client{Transport: economyRoundTrip(func(r *http.Request) (*http.Response, error) {
		started <- r.URL.Host
		if r.URL.Host == "api.ipify.org" {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ip":"203.0.113.9"}`)), Header: make(http.Header)}, nil
	})}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if ip, err := (Container{}).observeGatewayExitIP(ctx, g); err != nil || ip != "203.0.113.9" {
		t.Fatalf("parallel fallback missing ip=%s err=%v", ip, err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("parallel observer not restored")
		}
	}
}

func TestEconomyRejectedStickyCandidateClosesSocket(t *testing.T) {
	closed := make(chan struct{}, 1)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"ip":"203.0.113.9","country":"Other","country_code":"xx","timezone":{"id":"UTC"}}`)
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateClosed {
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	}
	srv.Start()
	defer srv.Close()
	target, _ := url.Parse(srv.URL)
	raw := &http.Transport{}
	g := &network.Gateway{Transport: raw, Client: &http.Client{Transport: economyRoundTrip(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		u := *r.URL
		clone.URL = &u
		clone.URL.Scheme = target.Scheme
		clone.URL.Host = target.Host
		return raw.RoundTrip(clone)
	})}}
	candidate, err := observeStickyCandidate(context.Background(), g)
	if err != nil || candidate.CountryCode != "xx" {
		t.Fatalf("candidate=%+v err=%v", candidate, err)
	}
	// The caller rejects the country before performing collision/port operations.
	// Its socket must already be closed, without depending on any caller cleanup.
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("rejected candidate leaked an idle proxy socket")
	}
}
