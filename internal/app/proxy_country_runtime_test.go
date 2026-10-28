package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/lib/pq"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxyselection"
)

func countryFixture(t *testing.T, db *sql.DB, countries string) (Container, string, []string) {
	t.Helper()
	var a string
	if err := db.QueryRow(`INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'digitalocean','country fixture','unused') RETURNING id::text`).Scan(&a); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 2)
	for i := range ids {
		if err := db.QueryRow(`INSERT INTO proxies(id,name,type,host,port,status,adapter,username) VALUES(gen_random_uuid(),'fixture-'||gen_random_uuid()::text,'http','127.0.0.1',1080,'healthy','suffix-session','base') RETURNING id::text`).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	execBootstrap(t, db, `INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,'proxy_required',$2)`, a, ids[0])
	for i, p := range ids {
		execBootstrap(t, db, `INSERT INTO account_proxy_pool(account_id,proxy_id,priority) VALUES($1,$2,$3)`, a, p, i)
	}
	execBootstrap(t, db, `INSERT INTO account_network_identities(account_id,preferred_country_code,preferred_country,timezone,locale) VALUES($1,'it','Italy','Europe/Rome','it-IT')`, a)
	if countries != "" {
		execBootstrap(t, db, `INSERT INTO account_proxy_country_policies(account_id,countries) VALUES($1,$2)`, a, countries)
	}
	return Container{DB: db, Accounts: Repository{DB: db}, Economy: &network.EconomyController{DB: db}}, a, ids
}
func countryProof(proxy, country, ip string) countryCandidate {
	return countryCandidate{ProxyID: proxy, Session: "candidate-session", Port: 10081, Target: country, Geo: stickyGeo{IP: ip, Country: country, CountryCode: strings.ToLower(country), Timezone: "America/New_York", ASN: "AS64500 Fixture"}, FirstSeen: time.Now().Add(-20 * time.Second)}
}

func TestCountryRuntimeAtomicPublicationAndFencesPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c, a, ps := countryFixture(t, db, `["US","TR","AUTO"]`)
	if ready, err := countryPolicyReady(ctx, db, a); err != nil || ready {
		t.Fatal("unproved policy admitted", ready, err)
	}
	s, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	v := countryProof(ps[1], "US", "198.51.100.19")
	if err = c.finishCountryWork(ctx, s, &v, nil, "ready", "fixture", true); err != nil {
		t.Fatal(err)
	}
	if err = c.requireAccountNetworkReady(ctx, a); err != nil {
		t.Fatal(err)
	}
	cfg, err := c.Accounts.Account(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Proxy.ID != ps[1] || cfg.Proxy.Port != 10081 || !strings.Contains(cfg.ProxyUsername, "cr.us;sid.candidate-session") {
		t.Fatal("committed route differs from proof", cfg.Proxy, cfg.ProxyUsername)
	}
	var learned string
	var generation, epoch int64
	if err = db.QueryRow(`SELECT i.preferred_country_code,r.generation,t.transport_epoch FROM account_network_identities i JOIN proxy_runtime_state r USING(account_id) JOIN account_transport_state t USING(account_id) WHERE i.account_id=$1 AND r.proxy_id=$2`, a, ps[1]).Scan(&learned, &generation, &epoch); err != nil {
		t.Fatal(err)
	}
	if learned != "it" || generation < 1 || epoch < 1 {
		t.Fatal("legacy preference lost or no cache fence", learned, generation, epoch)
	}

	rt := AccountRuntime{Config: cfg, TransportGeneration: generation, TransportEpoch: epoch}
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	ready, e := providerObservationNetworkReady(ctx, tx, rt)
	tx.Rollback()
	if e != nil || !ready {
		t.Fatal("verified policy not publishable", ready, e)
	}
	execBootstrap(t, db, `UPDATE account_proxy_selection SET applied_revision=0 WHERE account_id=$1`, a)
	tx, e = db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	ready, e = providerObservationNetworkReady(ctx, tx, rt)
	tx.Rollback()
	if e != nil || ready {
		t.Fatal("pending country policy published provider READY", ready, e)
	}
	execBootstrap(t, db, `UPDATE account_proxy_selection SET applied_revision=1 WHERE account_id=$1`, a)
	execBootstrap(t, db, `UPDATE proxies SET status='down' WHERE id=$1`, ps[1])
	if err = c.ensureActiveAccountProxy(ctx, a); !errors.Is(err, ErrNetworkNotReady) {
		t.Fatal("legacy pool tried to replace explicit-country route", err)
	}
	var active string
	if err = db.QueryRow(`SELECT proxy_id::text FROM network_profiles WHERE account_id=$1`, a).Scan(&active); err != nil || active != ps[1] {
		t.Fatal("legacy selector changed ordered route", active, err)
	}
	execBootstrap(t, db, `UPDATE proxies SET status='healthy' WHERE id=$1`, ps[1])
	// A new keeper fences a crashed/late predecessor even with identical settings.
	stale, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	current, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.finishCountryWork(ctx, stale, &v, nil, "ready", "stale", true); !errors.Is(err, ErrStaleAccountNetwork) {
		t.Fatal("late writer accepted", err)
	}
	execBootstrap(t, db, `UPDATE proxies SET username='replacement' WHERE id=$1`, ps[1])
	if err = c.finishCountryWork(ctx, current, &v, nil, "ready", "stale", true); !errors.Is(err, ErrStaleAccountNetwork) {
		t.Fatal("changed credentials accepted", err)
	}
	if ready, err := countryPolicyReady(ctx, db, a); err != nil || ready {
		t.Fatal("invalidated proof admitted", ready, err)
	}
	current, err = c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, `UPDATE account_proxy_pool SET enabled=false WHERE account_id=$1 AND proxy_id=$2`, a, ps[1])
	if err = c.finishCountryWork(ctx, current, &v, nil, "ready", "stale", true); !errors.Is(err, ErrStaleAccountNetwork) {
		t.Fatal("removed proxy accepted", err)
	}
	if _, err = db.Exec(`UPDATE account_network_identities SET last_health_ok=true WHERE account_id=$1`, a); err == nil {
		t.Fatal("old binary bypassed explicit policy")
	}
}

func TestCountryRuntimeIndependentClaimsAndCollisionPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c, a, pa := countryFixture(t, db, `["US"]`)
	_, b, pb := countryFixture(t, db, `["TR"]`)
	sa, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := c.claimCountryWork(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	va := countryProof(pa[0], "US", "203.0.113.18")
	vb := countryProof(pb[0], "TR", "203.0.113.19")
	vb.Port = 10082
	vb.Session = "independent-session-b"
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for _, item := range []struct {
		s countrySnapshot
		v countryCandidate
	}{{sa, va}, {sb, vb}} {
		wg.Add(1)
		go func(s countrySnapshot, v countryCandidate) {
			defer wg.Done()
			out <- c.finishCountryWork(ctx, s, &v, nil, "ready", "fixture", true)
		}(item.s, item.v)
	}
	wg.Wait()
	close(out)
	success := 0
	for err := range out {
		if err == nil {
			success++
		} else {
			var pg *pq.Error
			if !errors.As(err, &pg) || pg.Constraint != "account_network_identity_ipv4_24_unique" {
				t.Fatal("wrong rejection invariant", err)
			}
		}
	}
	if success != 1 {
		t.Fatalf("unique /24 must have exactly one atomic winner: %d", success)
	}
	var ready int
	if err = db.QueryRow(`SELECT count(*) FROM account_network_identities WHERE account_id IN($1,$2) AND last_health_ok`, a, b).Scan(&ready); err != nil || ready != 1 {
		t.Fatal(ready, err)
	}
	_, legacy, _ := countryFixture(t, db, "")
	s, err := c.claimCountryWork(ctx, legacy)
	if err != nil || !proxyselection.Legacy(s.Config.Countries) || s.Fence != 0 {
		t.Fatal("migration changed absent AUTO policy", s, err)
	}
}

func TestCountryRuntimePriorityStabilityAndNoChurnPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c, a, ps := countryFixture(t, db, `["US","TR","AUTO"]`)
	seen := []string{}
	usAvailable := false
	probe := func(_ context.Context, _ countrySnapshot, p countryProxy, v countryCandidate) (countryCandidate, proxyselection.Evaluation) {
		seen = append(seen, v.Target+":"+p.ID)
		if v.Target == "US" && (!usAvailable || p.ID == ps[0]) {
			return v, proxyselection.Evaluation{Health: proxyselection.Unusable, ExhaustProvider: true}
		}
		if v.FirstSeen.IsZero() {
			v.FirstSeen = time.Now()
		}
		v.Geo = stickyGeo{IP: "192.0.2.55", CountryCode: strings.ToLower(v.Target), Country: v.Target, Timezone: "Europe/Istanbul"}
		return v, proxyselection.Evaluation{Health: proxyselection.Usable, Candidate: v}
	}
	if err := c.maintainOrderedCountryWithProbe(ctx, a, probe); err != nil {
		t.Fatal(err)
	}
	s, err := loadCountrySnapshot(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	if s.Stage != "TR" || len(seen) != 3 || seen[0] != "US:"+ps[0] || seen[1] != "US:"+ps[1] {
		t.Fatal("lower country before healthy peers exhausted", seen, s.Stage)
	}
	originalSession := s.Session
	originalEpoch := s.Epoch
	seen = nil
	if err = c.maintainOrderedCountryWithProbe(ctx, a, probe); err != nil {
		t.Fatal(err)
	}
	s, err = loadCountrySnapshot(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	if s.Session != originalSession || s.Epoch != originalEpoch {
		t.Fatal("healthy fallback churned")
	}
	// Expire only negative country evidence; a higher healthy candidate must wait
	// for a spaced second verification while the incumbent remains usable.
	s.Checkpoint.Countries["US"].ExhaustedUntil = time.Now().Add(-time.Second)
	raw, _ := json.Marshal(s.Checkpoint)
	execBootstrap(t, db, `UPDATE account_proxy_selection SET checkpoint=$2 WHERE account_id=$1`, a, raw)
	usAvailable = true
	if err = c.maintainOrderedCountryWithProbe(ctx, a, probe); err != nil {
		t.Fatal(err)
	}
	s, err = loadCountrySnapshot(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	if s.Stage != "TR" || s.Candidate == nil || s.Candidate.Target != "US" || !s.Health {
		t.Fatal("unstable failback disturbed incumbent", s.Stage, s.Candidate, s.Health)
	}
	s.Candidate.FirstSeen = time.Now().Add(-11 * time.Second)
	raw, _ = json.Marshal(s.Candidate)
	execBootstrap(t, db, `UPDATE account_proxy_selection SET candidate=$2 WHERE account_id=$1`, a, raw)
	if err = c.maintainOrderedCountryWithProbe(ctx, a, probe); err != nil {
		t.Fatal(err)
	}
	s, err = loadCountrySnapshot(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	if s.Stage != "US" || s.Active != ps[1] || s.Session == originalSession || s.Candidate != nil {
		t.Fatal("stable higher-country failback not adopted", s.Stage, s.Active)
	}
	// Observer outage cannot authorize TR/AUTO; a verified incumbent is retained.
	unknown := func(_ context.Context, _ countrySnapshot, _ countryProxy, v countryCandidate) (countryCandidate, proxyselection.Evaluation) {
		return v, proxyselection.Evaluation{Health: proxyselection.Unknown}
	}
	if err = c.maintainOrderedCountryWithProbe(ctx, a, unknown); err != nil {
		t.Fatal(err)
	}
	s, err = loadCountrySnapshot(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	if s.Stage != "US" || s.Health {
		t.Fatal("unknown observer admitted or fell through", s.Stage, s.Health)
	}
}

func TestLegacyIdentityRejectsStaleConfigurationPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c, a, ps := countryFixture(t, db, "")
	execBootstrap(t, db, `UPDATE account_network_identities SET sticky_session='old',exit_ip='198.18.0.4',subnet_key='198.18.0.0/24',last_health_ok=true WHERE account_id=$1`, a)
	guarded, err := c.legacyIdentityContext(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.execLegacyIdentity(guarded, `UPDATE account_network_identities SET sticky_session='new',exit_ip='198.18.1.4',subnet_key='198.18.1.0/24' WHERE account_id=$1`, a); err != nil {
		t.Fatal("own identity update failed", err)
	}
	if _, err = c.execLegacyIdentity(guarded, `UPDATE account_network_identities SET last_health_ok=true WHERE account_id=$1`, a); err != nil {
		t.Fatal("own epoch advancement was lost", err)
	}
	execBootstrap(t, db, `UPDATE proxies SET username='changed-after-probe' WHERE id=$1`, ps[0])
	if _, err = c.execLegacyIdentity(guarded, `UPDATE account_network_identities SET exit_ip='198.18.2.4',subnet_key='198.18.2.0/24',last_health_ok=true WHERE account_id=$1`, a); !errors.Is(err, ErrStaleAccountNetwork) {
		t.Fatal("legacy stale observation published", err)
	}
	current, err := loadCountrySnapshot(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	if current.IP != "198.18.1.4" {
		t.Fatal("stale identity replaced live route", current.IP)
	}
	guarded, err = c.legacyIdentityContext(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, `UPDATE network_profiles SET proxy_id=$2 WHERE account_id=$1`, a, ps[1])
	if _, err = c.execLegacyIdentity(guarded, `UPDATE account_network_identities SET last_health_ok=true WHERE account_id=$1`, a); !errors.Is(err, ErrStaleAccountNetwork) {
		t.Fatal("legacy publication ignored proxy change", err)
	}
}

func TestCountryAutoCompatibilityLeaseAndRecoveryPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c, a, ps := countryFixture(t, db, `["US","AUTO"]`)
	execBootstrap(t, db, `UPDATE proxies SET status='down' WHERE id=$1`, ps[0])
	s, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	legacyCalls := 0
	legacy := func(ctx context.Context, account string) error {
		legacyCalls++
		if err := c.ensureActiveAccountProxy(ctx, account); err != nil {
			return err
		}
		guarded, err := c.legacyIdentityContext(ctx, account)
		if err != nil {
			return err
		}
		_, err = c.execLegacyIdentity(guarded, `UPDATE account_network_identities SET sticky_session='auto-session',exit_ip='198.18.20.2',subnet_key='198.18.20.0/24',country='Italy',country_code='it',last_health_ok=true,last_health_at=now(),rotation_started_at=NULL WHERE account_id=$1`, account)
		return err
	}
	if err = c.maintainCountryAutoWithLegacy(ctx, s, legacy); !errors.Is(err, ErrNetworkNotReady) || legacyCalls != 0 {
		t.Fatal("AUTO ran without higher-tier exhaustion", err, legacyCalls)
	}
	s.Checkpoint = proxyselection.Checkpoint{Scope: s.Scope, Countries: map[string]*proxyselection.Sweep{"US": {Started: time.Now(), ExhaustedUntil: time.Now().Add(time.Minute)}}}
	if err = c.maintainCountryAutoWithLegacy(ctx, s, legacy); err != nil {
		t.Fatal(err)
	}
	current, err := loadCountrySnapshot(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	if current.Active != ps[1] || current.Stage != "AUTO" || current.Applied != 1 || current.Target != "" || current.LearnedCountry != "it" || !current.Health {
		t.Fatal("legacy AUTO intent/recovery changed", current.Active, current.Stage, current.Applied, current.Target, current.LearnedCountry, current.Health)
	}
	var lease bool
	if err = db.QueryRow(`SELECT legacy_lease_until IS NOT NULL FROM account_proxy_selection WHERE account_id=$1`, a).Scan(&lease); err != nil || lease {
		t.Fatal("compatibility writer lease left open", lease, err)
	}
	if _, err = db.Exec(`UPDATE account_network_identities SET sticky_session='late-old-worker' WHERE account_id=$1`, a); err == nil {
		t.Fatal("late legacy writer survived lease closure")
	}
	if ready, err := countryPolicyReady(ctx, db, a); err != nil || !ready {
		t.Fatal("valid AUTO proof not admitted", ready, err)
	}
}
