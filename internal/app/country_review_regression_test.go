package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxycontrol"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxyselection"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
)

func TestCountryCredentialSnapshotBlocksBeforeGatewayPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c, a, ps := countryFixture(t, db, `["US"]`)
	var err error
	c.Secrets, err = secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Secrets.PutProxy(ctx, ps[0], "proxy-password", "proxy_password", []byte("fixture-old")); err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, `UPDATE proxies SET secret_ref='proxy-password' WHERE id=$1`, ps[0])
	s, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	p := s.Config.Pool[0]
	if err = c.Secrets.PutProxy(ctx, ps[0], "proxy-password", "proxy_password", []byte("fixture-new")); err != nil {
		t.Fatal(err)
	}
	var calls int
	_, result := c.verifyCountryCandidateWithGateway(ctx, s, p, countryCandidate{Target: "US"}, func(context.Context, string, network.Proxy, network.ProxyCredentials) (*network.Gateway, error) {
		calls++
		return nil, errors.New("must not send")
	})
	if calls != 0 || result.Health != proxyselection.Unknown {
		t.Fatal("new key escaped stale snapshot", calls, result)
	}
	if value, err := c.Secrets.GetProxyAtVersion(ctx, p.ID, p.SecretRef, p.SecretVersion); !errors.Is(err, secrets.ErrProxySecretVersionChanged) || len(value) != 0 {
		t.Fatal("stale key version read", err)
	}
}

func TestCountrySavedCandidateConsumedOncePostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c, a, ps := countryFixture(t, db, `["US"]`)
	s, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	s.Checkpoint.Scope = s.Scope
	v := countryProof(ps[0], "US", "198.18.81.3")
	if err = c.finishCountryWork(ctx, s, nil, &v, "stabilizing", "fixture", false); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	saved := 0
	probe := func(_ context.Context, _ countrySnapshot, p countryProxy, v countryCandidate) (countryCandidate, proxyselection.Evaluation) {
		key := p.ID + ":" + v.Session
		if seen[key] {
			t.Fatalf("candidate retried as a new session: %s", key)
		}
		seen[key] = true
		if v.Session == "candidate-session" {
			saved++
		}
		return v, proxyselection.Evaluation{Health: proxyselection.Unusable}
	}
	if err = c.maintainOrderedCountryWithProbe(ctx, a, probe); err != nil {
		t.Fatal(err)
	}
	if saved != 1 || len(seen) != 12 {
		t.Fatal("provider candidate budget not consumed independently", saved, len(seen))
	}
}

func TestCountryGuardOneConnectionAndStreamingPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c, a, ps := countryFixture(t, db, `["US"]`)
	s, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	v := countryProof(ps[0], "US", "198.18.82.3")
	if err = c.finishCountryWork(ctx, s, &v, nil, "ready", "fixture", true); err != nil {
		t.Fatal(err)
	}
	s, err = loadCountrySnapshot(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	state, err := (proxycontrol.SQLStore{DB: db}).Load(ctx, a, ps[0], "digitalocean")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	var uploaded io.ReadCloser
	var observations atomic.Int32
	var observationErr error
	tr := accountNetworkGuardTransport{Container: c, AccountID: a, Provider: "digitalocean", ProxyID: ps[0], Generation: state.Generation, TransportEpoch: s.Epoch, StickySession: s.Session, ExitIP: s.IP,
		Base: countryTransportFunc(func(r *http.Request) (*http.Response, error) {
			uploaded = r.Body
			return &http.Response{StatusCode: 204, Header: make(http.Header), Body: http.NoBody}, nil
		}),
		Observe: func(network.TransportObservation) {
			report, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			var one int
			observationErr = db.QueryRowContext(report, `SELECT 1`).Scan(&one)
			observations.Add(1)
		}}
	requestCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(requestCtx, http.MethodPost, "https://example.invalid", io.NopCloser(strings.NewReader("unfinished upload")))
	if _, err = tr.RoundTrip(req); err != nil {
		t.Fatal("guard exhausted its own SQL pool", err)
	}
	if observations.Load() != 0 || db.Stats().InUse != 1 {
		t.Fatal("released route at early headers", observations.Load(), db.Stats())
	}
	if err = uploaded.Close(); err != nil {
		t.Fatal(err)
	}
	if observations.Load() != 1 || observationErr != nil || db.Stats().InUse != 0 {
		t.Fatal("observer ran under route connection or release missing", observations.Load(), observationErr, db.Stats())
	}
	// A nonempty response body also pins the connection until consumption/close.
	tr.Base = countryTransportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("response"))}, nil
	})
	req, _ = http.NewRequestWithContext(requestCtx, http.MethodGet, "https://example.invalid", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if observations.Load() != 1 || db.Stats().InUse != 1 {
		t.Fatal("response stream escaped route lock")
	}
	if _, err = io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if observations.Load() != 2 || observationErr != nil || db.Stats().InUse != 0 {
		t.Fatal("response completion did not release exactly once", observationErr, db.Stats())
	}
}

func TestCountryExpiredProofPreservesIncumbentPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c, a, ps := countryFixture(t, db, `["US","TR"]`)
	s, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	v := countryProof(ps[0], "US", "198.18.83.3")
	if err = c.finishCountryWork(ctx, s, &v, nil, "ready", "fixture", true); err != nil {
		t.Fatal(err)
	}
	s, err = c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	expired := countryProof(ps[1], "TR", "198.18.84.3")
	expired.FirstSeen = time.Now().Add(-time.Minute)
	s.Checkpoint.Countries = map[string]*proxyselection.Sweep{"US": {ExhaustedUntil: time.Now().Add(time.Minute)}}
	if err = c.finishCountryWork(ctx, s, &expired, nil, "ready", "expired", true); !errors.Is(err, ErrNetworkNotReady) {
		t.Fatal("stale observation published", err)
	}
	latest, err := loadCountrySnapshot(ctx, db, a)
	if err != nil || latest.IP != v.Geo.IP || !latest.Health {
		t.Fatal("expired replacement damaged incumbent", latest.IP, latest.Health, err)
	}
}

func TestCountrySharedProxyInvalidationOrderPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c, a, pa := countryFixture(t, db, `["US"]`)
	_, b, pb := countryFixture(t, db, `["TR"]`)
	for _, x := range []struct{ a, p string }{{a, pb[0]}, {b, pa[0]}} {
		execBootstrap(t, db, `INSERT INTO account_proxy_pool(account_id,proxy_id,priority) VALUES($1,$2,9)`, x.a, x.p)
	}
	if _, err := c.claimCountryWork(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := c.claimCountryWork(ctx, b); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, p := range []string{pa[0], pb[0]} {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			<-start
			timeout, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			_, err := db.ExecContext(timeout, `UPDATE proxies SET username=username||'-new' WHERE id=$1`, p)
			errs <- err
		}(p)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("shared proxy invalidation deadlocked", err)
		}
	}
}

type independentCountryDriver struct{ genericProxyAdapter }

func (independentCountryDriver) Name() string { return "independent-country-fixture" }
func (independentCountryDriver) Capabilities() ProxyCapabilities {
	return ProxyCapabilities{StickySession: true}
}
func (independentCountryDriver) Username(r ProxySessionRequest) string {
	if r.TargetCountry {
		return "unsupported-country-target"
	}
	return r.BaseUsername + "|" + r.SessionID
}
func TestCountryIndependentDriverContractPostgres(t *testing.T) {
	driver := independentCountryDriver{}
	if err := RegisterProxyAdapter(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { proxyDrivers.Lock(); delete(proxyDrivers.adapters, driver.Name()); proxyDrivers.Unlock() })
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c, a, ps := countryFixture(t, db, "")
	execBootstrap(t, db, `UPDATE proxies SET adapter=$2 WHERE id=$1`, ps[0], driver.Name())
	execBootstrap(t, db, `UPDATE account_network_identities SET sticky_session='existing-session',sticky_port=10083 WHERE account_id=$1`, a)
	cfg, session, cc, _, _, _, err := c.stickyConfig(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Proxy.Port != 1080 || cfg.ProxyBaseUsername != "base" || cfg.ProxyUsername != "base|existing-session" {
		t.Fatal("driver inherited another grammar or dedicated port", cfg.Proxy.Port, cfg.ProxyBaseUsername, cfg.ProxyUsername)
	}
	if got := proxySessionUsername(cfg.ProxyAdapter, cfg.ProxyBaseUsername, cc, session, true); got != "base|existing-session" {
		t.Fatal("non-idempotent driver applied twice or country target forced", got)
	}
	snapshot, err := loadCountrySnapshot(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	p := snapshot.Config.Pool[0]
	seen := ""
	factory := func(_ context.Context, _ string, p network.Proxy, credentials network.ProxyCredentials) (*network.Gateway, error) {
		if p.Port != 1080 {
			t.Fatal("driver received dedicated port", p.Port)
		}
		seen = credentials.Username
		return &network.Gateway{Client: &http.Client{Transport: countryTransportFunc(func(r *http.Request) (*http.Response, error) {
			value := `{"ip":"198.18.85.3"}`
			if r.URL.Hostname() == "ipwho.is" {
				value = `{"ip":"198.18.85.3","country":"United States","country_code":"US","timezone":{"id":"America/New_York"}}`
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(value)), Header: make(http.Header), Request: r}, nil
		})}}, nil
	}
	_, result := c.verifyCountryCandidateWithGateway(ctx, snapshot, p, countryCandidate{ProxyID: p.ID, Target: "US", Session: "new-session", Port: p.Port}, factory)
	if result.Health != proxyselection.Usable || seen != "base|new-session" {
		t.Fatal("independent driver country proof failed", seen, result)
	}
}

func TestCountryAcquirePreservesHealthHistoryPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	_, a, ps := countryFixture(t, db, "")
	p := ps[0]
	provider := "digitalocean"
	ctx := context.Background()
	store := proxycontrol.SQLStore{DB: db}
	now := time.Now().UTC()
	state := proxycontrol.DefaultState(a, p, provider)
	state.HealthState = network.StatusDegraded
	state.ConsecutiveFailures = 2
	state.ConsecutiveSuccesses = 4
	state.LastErrorClass = "fixture"
	state.LastErrorDetail = "retained"
	state.LastCheckedAt = &now
	state.LastSuccessAt = &now
	if err := store.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	got, allowed, err := store.Acquire(ctx, a, p, provider, now, time.Second)
	if err != nil || !allowed || got.ConsecutiveFailures != 2 || got.ConsecutiveSuccesses != 4 || got.LastErrorClass != "fixture" || got.LastErrorDetail != "retained" || got.LastCheckedAt == nil || got.LastSuccessAt == nil || got.HealthState != network.StatusDegraded {
		t.Fatal("Acquire reset persisted health history", got, allowed, err)
	}
}
