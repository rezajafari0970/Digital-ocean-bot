package app

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestExplicitCountryCannotUseDirectRuntimePostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	c, a, _ := countryFixture(t, db, `["US"]`)
	execBootstrap(t, db, `UPDATE network_profiles SET mode='direct',proxy_id=NULL WHERE account_id=$1`, a)
	if err := c.requireAccountNetworkReady(context.Background(), a); err == nil {
		t.Fatal("explicit country admitted direct mode")
	}
	cfg, err := c.Accounts.Account(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	base := &countRoundTripper{}
	guard := accountNetworkGuardTransport{Base: base, Container: c, AccountID: a, Provider: cfg.Provider, Direct: true, TransportEpoch: cfg.NetworkTransportEpoch, RouteFingerprint: cfg.ProxyRouteFingerprint}
	if _, err := guard.RoundTrip(httptest.NewRequest("GET", "https://provider.fixture", nil)); err == nil {
		t.Fatal("direct guard bypassed explicit country")
	}
	if base.calls.Load() != 0 {
		t.Fatal("provider credential dispatched directly")
	}
	if _, _, _, err := c.OnboardingClient(context.Background(), "", "US"); err == nil {
		t.Fatal("onboarding country downgraded to direct")
	}
	if _, _, _, err := c.OnboardingClient(context.Background(), "", "ZZ"); err == nil {
		t.Fatal("unknown country admitted")
	}
}
