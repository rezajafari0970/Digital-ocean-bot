package app

import (
	"context"
	"crypto/tls"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxycontrol"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxyselection"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCountryReviewTwoSnapshotCannotAdoptFreshFencePostgres(t *testing.T) {
	for _, kind := range []string{"session", "endpoint"} {
		t.Run(kind, func(t *testing.T) {
			db := installerBootstrapDB(t)
			ctx := context.Background()
			c, a, ps := countryFixture(t, db, `["US"]`)
			s, err := c.claimCountryWork(ctx, a)
			if err != nil {
				t.Fatal(err)
			}
			v := countryProof(ps[0], "US", "198.18.91.5")
			if err = c.finishCountryWork(ctx, s, &v, nil, "ready", "fixture", true); err != nil {
				t.Fatal(err)
			}
			cfg, err := c.Accounts.Account(ctx, a)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "endpoint" {
				execBootstrap(t, db, `UPDATE proxies SET host='127.0.0.2' WHERE id=$1`, ps[0])
			} else {
				v.Session = "next-session"
			}
			s, err = c.claimCountryWork(ctx, a)
			if err != nil {
				t.Fatal(err)
			}
			if err = c.finishCountryWork(ctx, s, &v, nil, "ready", "fresh", true); err != nil {
				t.Fatal(err)
			}
			rt, err := c.buildProviderNetworkRuntime(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer rt.CloseIdleConnections()
			guard, ok := rt.Client.Transport.(accountNetworkGuardTransport)
			if !ok {
				t.Fatal("missing request guard")
			}
			base := &guardCountTransport{}
			guard.Base = base
			guard.NativeWriteTrace = false
			req, _ := http.NewRequestWithContext(ctx, "POST", "https://example.invalid/create", nil)
			if _, err = guard.RoundTrip(req); !errors.Is(err, ErrStaleAccountNetwork) {
				t.Fatal("old parameters blessed with fresh fence", err)
			}
			if base.calls.Load() != 0 {
				t.Fatal("stale route dispatched")
			}
		})
	}
}

type delayedNativeUpload struct {
	entered, release, closed chan struct{}
	once, closeOnce          sync.Once
}

func (b *delayedNativeUpload) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return 0, io.EOF
}

// Model a native upload which has not yet settled after source Close returns.
func (b *delayedNativeUpload) Close() error { b.closeOnce.Do(func() { close(b.closed) }); return nil }
func TestCountryReviewTwoNativeHTTP2UploadFencePostgres(t *testing.T) {
	for _, mode := range []string{"response-eof", "response-close", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			db := installerBootstrapDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			c, a, ps := countryFixture(t, db, `["US"]`)
			s, err := c.claimCountryWork(ctx, a)
			if err != nil {
				t.Fatal(err)
			}
			v := countryProof(ps[0], "US", "198.18.92.5")
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
			body := &delayedNativeUpload{entered: make(chan struct{}), release: make(chan struct{}), closed: make(chan struct{})}
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(body.release) }) }
			defer release()
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.ProtoMajor != 2 {
					t.Error("native HTTP/2 required")
				}
				w.Header().Set("Content-Length", "2")
				w.WriteHeader(200)
				w.Write([]byte("ok"))
				w.(http.Flusher).Flush()
			}))
			server.EnableHTTP2 = true
			server.StartTLS()
			defer func() { release(); server.Close() }()
			native := server.Client().Transport.(*http.Transport).Clone()
			native.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
			native.ForceAttemptHTTP2 = true
			defer native.CloseIdleConnections()
			observed := make(chan struct{}, 1)
			guard := accountNetworkGuardTransport{Container: c, AccountID: a, Provider: "digitalocean", ProxyID: ps[0], Generation: state.Generation, TransportEpoch: s.Epoch, StickySession: s.Session, ExitIP: s.IP, NativeWriteTrace: true, Base: native, Observe: func(network.TransportObservation) { observed <- struct{}{} }}
			requestCtx, requestCancel := context.WithCancel(ctx)
			defer requestCancel()
			req, _ := http.NewRequestWithContext(requestCtx, "POST", server.URL, body)
			resp, err := guard.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			select {
			case <-body.entered:
			case <-ctx.Done():
				t.Fatal("upload never started")
			}
			switch mode {
			case "response-eof":
				if _, err = io.ReadAll(resp.Body); err != nil {
					t.Fatal(err)
				}
			case "response-close":
				resp.Body.Close()
			case "cancel":
				requestCancel()
			}
			select {
			case <-observed:
				t.Fatal("released before native writer settled")
			case <-time.After(80 * time.Millisecond):
			}
			if mode == "response-eof" {
				select {
				case <-requestCtx.Done():
					t.Fatal("response EOF canceled valid upload")
				default:
				}
			}
			writer := make(chan error, 1)
			go func() {
				tx, e := countryRouteTx(ctx, db, strings.ToUpper(a))
				if e == nil {
					e = tx.Commit()
				}
				writer <- e
			}()
			select {
			case e := <-writer:
				t.Fatal("route writer bypassed in-flight native upload", e)
			case <-time.After(80 * time.Millisecond):
			}
			release()
			select {
			case <-observed:
			case <-ctx.Done():
				t.Fatal("native upload did not release route")
			}
			select {
			case e := <-writer:
				if e != nil {
					t.Fatal(e)
				}
			case <-ctx.Done():
				t.Fatal("route writer never resumed")
			}
		})
	}
}

func TestCountryReviewTwoPublicKeeperOneConnectionPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	c, a, _ := countryFixture(t, db, `["US"]`)
	execBootstrap(t, db, `UPDATE network_profiles SET mode='direct',proxy_id=NULL WHERE account_id=$1`, a)
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.MaintainProxyControlPlane(ctx, strings.ToUpper(a)); err != nil {
		t.Fatal("keeper pinned its only SQL connection", err)
	}
	var owner *string
	if err := db.QueryRow(`SELECT keeper_owner FROM account_proxy_selection WHERE account_id=$1`, a).Scan(&owner); err != nil || owner != nil {
		t.Fatal("lease not released", owner, err)
	}
}

func TestCountryReviewTwoPassiveHealthCannotBeOverwrittenPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	_, a, ps := countryFixture(t, db, "")
	store := proxycontrol.SQLStore{DB: db}
	old, allowed, err := store.Acquire(ctx, a, ps[0], "digitalocean", time.Now(), time.Second)
	if err != nil || !allowed {
		t.Fatal(allowed, err)
	}
	for range 3 {
		if _, err = store.ApplyObservation(ctx, a, ps[0], "digitalocean", network.HealthResult{Status: network.StatusDown, CheckedAt: time.Now(), Error: "proxy authentication required"}, proxycontrol.DefaultPolicy()); err != nil {
			t.Fatal(err)
		}
	}
	before, err := store.Load(ctx, a, ps[0], "digitalocean")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	old.HealthState = network.StatusDegraded
	if err = proxycontrol.SaveStateTx(ctx, tx, old); !errors.Is(err, proxycontrol.ErrStateConflict) {
		t.Fatal("stale keeper overwrote passive failures", err)
	}
	tx.Rollback()
	after, err := store.Load(ctx, a, ps[0], "digitalocean")
	if err != nil || after.ConsecutiveFailures != before.ConsecutiveFailures || after.CircuitState != before.CircuitState || after.ConsecutiveFailures < 3 {
		t.Fatal("new evidence lost", after, err)
	}
}

func TestCountryReviewTwoProofExpiresDuringAllocationPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	c, a, ps := countryFixture(t, db, `["US","TR"]`)
	s, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	v := countryProof(ps[0], "US", "198.18.93.5")
	if err = c.finishCountryWork(ctx, s, &v, nil, "ready", "fixture", true); err != nil {
		t.Fatal(err)
	}
	s, err = c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	s.Checkpoint.Countries = map[string]*proxyselection.Sweep{"US": {ExhaustedUntil: time.Now().Add(600 * time.Millisecond)}}
	before, err := loadCountrySnapshot(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	blocker, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	if _, err = blocker.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended('sticky-port-allocation',0))`); err != nil {
		t.Fatal(err)
	}
	defer blocker.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtextextended('sticky-port-allocation',0))`)
	done := make(chan error, 1)
	candidate := countryProof(ps[1], "TR", "198.18.94.5")
	go func() { done <- c.finishCountryWork(ctx, s, &candidate, nil, "ready", "late", true) }()
	deadline := time.Now().Add(time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		if err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted AND classid::bigint=((hashtextextended('sticky-port-allocation',0)>>32)&4294967295) AND objid::bigint=(hashtextextended('sticky-port-allocation',0)&4294967295))`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("publication never reached allocation lock")
	}
	<-time.After(time.Until(s.Checkpoint.Countries["US"].ExhaustedUntil) + 20*time.Millisecond)
	if _, err = blocker.ExecContext(ctx, `SELECT pg_advisory_unlock(hashtextextended('sticky-port-allocation',0))`); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, ErrNetworkNotReady) {
		t.Fatal("expired proof committed", err)
	}
	after, err := loadCountrySnapshot(ctx, db, a)
	if err != nil || before.Active != after.Active || before.IP != after.IP || before.Session != after.Session || before.Epoch != after.Epoch || before.Applied != after.Applied || !after.Health {
		t.Fatal("failed transaction changed incumbent", after, err)
	}
}

func TestCountryReviewTwoMixedObserverFailureStaysUnknownPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c, a, _ := countryFixture(t, db, `["US","TR"]`)
	s, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	p := s.Config.Pool[0]
	for _, mode := range []string{"status", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			factory := func(context.Context, string, network.Proxy, network.ProxyCredentials) (*network.Gateway, error) {
				return &network.Gateway{Client: &http.Client{Transport: countryTransportFunc(func(req *http.Request) (*http.Response, error) {
					if req.URL.Hostname() == "ipwho.is" {
						return nil, errors.New("fixture TLS observer failure")
					}
					status := 200
					body := `{"ip":"invalid"}`
					if mode == "status" {
						status = 503
						body = "observer unavailable"
					}
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
				})}}, nil
			}
			_, result := c.verifyCountryCandidateWithGateway(ctx, s, p, countryCandidate{ProxyID: p.ID, Target: "US", Session: "fixture", Port: p.Port}, factory)
			if result.Health != proxyselection.Unknown || result.ExhaustProvider {
				t.Fatal("indeterminate observers exhausted country", result)
			}
		})
	}
}

func TestCountryReviewTwoExistingGenericAutoSurvivesHigherUnknownPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c, a, ps := countryFixture(t, db, `["US","AUTO"]`)
	execBootstrap(t, db, `UPDATE proxies SET adapter='generic' WHERE id=$1`, ps[0])
	s, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	s.Checkpoint.Scope = s.Scope
	s.Checkpoint.Countries = map[string]*proxyselection.Sweep{"US": {ExhaustedUntil: time.Now().Add(time.Minute)}}
	v := countryProof(ps[0], "", "198.18.95.5")
	v.Session = ""
	v.Port = 1080
	v.Geo.CountryCode = "it"
	if err = c.finishCountryWork(ctx, s, &v, nil, "ready", "existing AUTO", true); err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, `UPDATE account_proxy_selection SET checkpoint='{}' WHERE account_id=$1`, a)
	factory := func(context.Context, string, network.Proxy, network.ProxyCredentials) (*network.Gateway, error) {
		return &network.Gateway{Client: &http.Client{Transport: countryTransportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ip":"198.18.96.5","country":"Italy","country_code":"IT","timezone":{"id":"Europe/Rome"},"connection":{"asn":64500,"org":"fixture"}}`))}, nil
		})}}, nil
	}
	err = c.maintainOrderedCountryWithProbe(ctx, a, func(ctx context.Context, s countrySnapshot, p countryProxy, v countryCandidate) (countryCandidate, proxyselection.Evaluation) {
		if v.Target != "" {
			return v, proxyselection.Evaluation{Health: proxyselection.Unknown}
		}
		return c.verifyCountryCandidateWithGateway(ctx, s, p, v, factory)
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := loadCountrySnapshot(ctx, db, a)
	if err != nil || !after.Health || after.IP != "198.18.96.5" || after.Applied != 1 || after.Stage != "AUTO" || after.Target != "" || after.Session != "" || after.LearnedCountry != "it" {
		t.Fatal("unknown higher tier destroyed existing AUTO", after, err)
	}
}
