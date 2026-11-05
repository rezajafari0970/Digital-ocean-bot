package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"strings"
	"testing"
	"time"
)

func TestProviderFailureReceiptPreservesSevenProviderContexts(t *testing.T) {
	for _, provider := range []string{"digitalocean", "vultr", "linode", "ovh", "aws", "upcloud", "kamatera"} {
		t.Run(provider, func(t *testing.T) {
			err := &providers.Error{Class: providers.ErrorAuthentication, StatusCode: 401, Code: "AUTHENTICATION_FAILED", Operation: "account", Message: "Rejected token=never-display-this", RetryAfter: 3 * time.Second}
			raw := providerFailureReceipt(provider, err, "TOKEN_INVALID", err.Error())
			var d struct{ Diagnostic ProviderFailureEvidence }
			if json.Unmarshal([]byte(raw), &d) != nil || d.Diagnostic.HTTPStatus != 401 || d.Diagnostic.Operation != "account" || d.Diagnostic.Provider != provider || d.Diagnostic.Guidance == "" {
				t.Fatal(raw)
			}
			if strings.Contains(raw, "never-display-this") {
				t.Fatal("credential leaked", raw)
			}
		})
	}
	if got := safeFailureOperation("/servers/sensitive-id?token=secret"); strings.Contains(got, "sensitive") || strings.Contains(got, "secret") {
		t.Fatal(got)
	}
}
func TestProviderFailureHistoryIsAtomicAndDeduplicated(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	rt := observationFixture(t, db)
	ctx := context.Background()
	cause := &providers.Error{Class: providers.ErrorAuthentication, StatusCode: 401, Code: "AUTHENTICATION_FAILED", Operation: "/account", Message: "Authentication rejected"}
	for i := 0; i < 2; i++ {
		if ok, e := c.RecordProviderObservation(ctx, observationAttempt(t, c, rt), ProviderStateTokenInvalid, cause, cause.Error()); !ok || e != nil {
			t.Fatal(ok, e)
		}
	}
	var n int
	var raw string
	if e := db.QueryRow("SELECT count(*) FROM audit_events WHERE account_id=$1 AND action='provider.observation.failed'", rt.Config.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if e := db.QueryRow("SELECT provider_state_detail FROM accounts WHERE id=$1", rt.Config.ID).Scan(&raw); e != nil || !strings.Contains(raw, `"http_status":401`) {
		t.Fatal(raw, e)
	}
	execBootstrap(t, db, `CREATE FUNCTION reject_diagnostic() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture'; END $$;CREATE TRIGGER reject_diagnostic BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_diagnostic()`)
	if ok, e := c.RecordProviderObservation(ctx, observationAttempt(t, c, rt), ProviderStateProxyError, errors.New("route failed"), "route failed"); ok || e == nil {
		t.Fatal("history write failure accepted")
	}
	var state string
	db.QueryRow("SELECT COALESCE(provider_error_state,'') FROM accounts WHERE id=$1", rt.Config.ID).Scan(&state)
	if state != "" {
		t.Fatal("state escaped failed history transaction")
	}
}
