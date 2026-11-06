package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxyselection"
)

func TestBrowserProxyHealthPinPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	c, a, ps := countryFixture(t, db, `["US"]`)
	ctx := context.Background()
	s, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	v := countryProof(ps[0], "US", "198.18.121.4")
	if err = c.finishCountryWork(ctx, s, &v, nil, "ready", "fixture", true); err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, `UPDATE proxy_economy_policy SET enabled=true,canary_account_id=NULL WHERE singleton`)
	execBootstrap(t, db, `UPDATE proxies SET last_success_at=now()-interval '7 minutes',last_checked_at=now()-interval '7 minutes' WHERE id=$1`, ps[0])
	err = c.requireAccountNetworkReady(ctx, a)
	if !errors.Is(err, ErrNetworkNotReady) || err.Error() != networkReadinessError("BASE_PROXY_HEALTH_STALE").Error() {
		t.Fatal("wrong freshness failure", err)
	}
	calls := 0
	factory := func(_ context.Context, _ string, _ network.Proxy, _ network.ProxyCredentials) (*network.Gateway, error) {
		return &network.Gateway{Client: &http.Client{Transport: countryTransportFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: r}, nil
		})}}, nil
	}
	client, closeRoute, _, err := c.onboardingClient(ctx, strings.ToUpper(ps[0]), "AUTO", factory)
	if err != nil {
		t.Fatal(err)
	}
	defer closeRoute()
	store := network.SQLHealthStore{DB: db}
	now := time.Now().UTC()
	health := network.HealthState{Status: network.StatusHealthy, LastCheckedAt: now, LastSuccessAt: now, ConsecutiveSuccesses: 3}
	if err = store.Save(ctx, ps[0], health); err != nil {
		t.Fatal("browser starved health publisher", err)
	}
	if err = c.requireAccountNetworkReady(ctx, a); err != nil {
		t.Fatal("fresh observation not admitted during browser session", err)
	}
	// Account Proxy test also publishes observed geo metadata on the shared base row.
	execBootstrap(t, db, `UPDATE proxies SET country='United States',country_code='us',asn='AS64500 fixture' WHERE id=$1`, ps[0])
	// The live health change must not release the configuration or secret pin.
	for _, query := range []string{
		`UPDATE proxies SET host='changed.invalid' WHERE id=$1`,
		`UPDATE proxies SET adapter='generic' WHERE id=$1`,
		`UPDATE proxies SET settings_revision=settings_revision+1 WHERE id=$1`,
		`DELETE FROM proxies WHERE id=$1`,
	} {
		_, err = db.Exec(query, ps[0])
		var locked *pq.Error
		if !errors.As(err, &locked) || locked.Code != "55P03" {
			t.Fatal("configuration escaped active pin", query, err)
		}
	}
	resp, err := client.Get("https://provider.fixture/account")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	health.Status = network.StatusDown
	health.LastError = "PROXY_AUTH_FAILED"
	if err = store.Save(ctx, ps[0], health); err != nil {
		t.Fatal(err)
	}
	if err = c.requireAccountNetworkReady(ctx, a); !errors.Is(err, ErrNetworkNotReady) {
		t.Fatal("failed proxy admitted", err)
	}
	if resp, err = client.Get("https://provider.fixture/account"); err == nil {
		resp.Body.Close()
		t.Fatal("failed proxy reached provider")
	}
	if calls != 1 {
		t.Fatal("blocked request escaped", calls)
	}
	closeRoute()
	if _, err = db.Exec(`UPDATE proxies SET host='after-close.invalid' WHERE id=$1`, ps[0]); err != nil {
		t.Fatal("configuration pin leaked", err)
	}
}

func TestPrimaryIdentityRecoversWithoutRotationPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	c, a, ps := countryFixture(t, db, `["US","TR","AUTO"]`)
	ctx := context.Background()
	s, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	v := countryProof(ps[0], "US", "198.18.122.4")
	if err = c.finishCountryWork(ctx, s, &v, nil, "ready", "fixture", true); err != nil {
		t.Fatal(err)
	}
	initial, err := loadCountrySnapshot(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	for cycle := 0; cycle < 12; cycle++ {
		calls := 0
		unknown := cycle == 2 || cycle == 3
		probe := func(_ context.Context, _ countrySnapshot, p countryProxy, x countryCandidate) (countryCandidate, proxyselection.Evaluation) {
			calls++
			if p.ID != initial.Active || x.Target != "US" || x.Session != initial.Session || x.Port != initial.Port || x.Geo.IP != initial.IP {
				t.Fatal("primary identity was replaced", cycle, x)
			}
			if unknown {
				return x, proxyselection.Evaluation{Health: proxyselection.Unknown}
			}
			x.Geo = v.Geo
			return x, proxyselection.Evaluation{Health: proxyselection.Usable, Candidate: x}
		}
		if err = c.maintainOrderedCountryWithProbe(ctx, a, probe); err != nil {
			t.Fatal(err)
		}
		current, err := loadCountrySnapshot(ctx, db, a)
		if err != nil {
			t.Fatal(err)
		}
		if calls != 1 || current.Stage != "US" || current.Session != initial.Session || current.IP != initial.IP || current.Epoch != initial.Epoch || current.Health == unknown {
			t.Fatal("primary continuity violated", cycle, calls, current.Health)
		}
		if unknown && !errors.Is(c.requireAccountNetworkReady(ctx, a), ErrNetworkNotReady) {
			t.Fatal("unknown identity admitted")
		}
	}
}

func TestNetworkReadinessDiagnosticsForEveryProvider(t *testing.T) {
	for _, provider := range []string{"digitalocean", "vultr", "linode", "ovh", "aws", "upcloud", "kamatera"} {
		cause := networkReadinessError("BASE_PROXY_HEALTH_STALE")
		var receipt struct {
			Diagnostic ProviderFailureEvidence `json:"diagnostic"`
		}
		raw := providerFailureReceipt(provider, cause, "PROXY_ERROR", cause.Error())
		if err := json.Unmarshal([]byte(raw), &receipt); err != nil {
			t.Fatal(err)
		}
		if receipt.Diagnostic.Code != string(cause) || receipt.Diagnostic.Operation != "network" || !strings.Contains(receipt.Diagnostic.Guidance, "monitor") || !errors.Is(cause, ErrNetworkNotReady) {
			t.Fatal(provider, raw)
		}
	}
}
