package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/serverprotection"
)

func TestProtectionCleanupReceiptIsDurableButEnabledTelemetryIsFresh(t *testing.T) {
	db := adminTestDB(t)
	_, _, panel := seedPanel(t, db, "http://cleanup.test:2053")
	ctx := context.Background()
	store := serverprotection.Store{DB: db}
	change := func(revision int64, enabled bool) {
		t.Helper()
		if _, err := store.Change(ctx, serverprotection.Request{RequestID: perfUUID(), ExpectedRevision: revision, Enabled: enabled, Scope: "fleet"}); err != nil {
			t.Fatal(err)
		}
	}
	check := func(fresh, cleaned bool) {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/v1/server-protection", nil)
		req = req.WithContext(context.WithValue(ctx, principalKey{}, auth.Principal{Role: auth.Admin}))
		w := httptest.NewRecorder()
		(&Server{DB: db}).serverProtectionStatus(w, req)
		var response struct {
			Panels []struct {
				ID              string
				Verified        bool
				CleanupVerified bool `json:"cleanup_verified"`
			}
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || len(response.Panels) != 1 {
			t.Fatal(w.Code, w.Body.String())
		}
		p := response.Panels[0]
		if p.ID != panel || p.Verified != fresh || p.CleanupVerified != cleaned {
			t.Fatalf("fresh=%v cleanup=%v, want %v/%v", p.Verified, p.CleanupVerified, fresh, cleaned)
		}
	}
	change(1, true)
	p := protectionPolicy(t, db, panel)
	if err := store.Receipt(ctx, panel, p, protectionReceipt(p), nil); err != nil {
		t.Fatal(err)
	}
	check(true, false)
	sqlMust(t, db, "UPDATE server_protection_nodes SET checked_at=now()-interval '2 minutes'")
	check(false, false) // Enabled telemetry must still expire.
	change(2, false)
	check(false, false) // Disable intent alone proves nothing.
	p = protectionPolicy(t, db, panel)
	if err := store.Receipt(ctx, panel, p, protectionReceipt(p), nil); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "UPDATE server_protection_nodes SET checked_at=now()-interval '2 minutes'")
	check(false, true) // A completed cleanup is not a fresh health observation.
	if err := store.Receipt(ctx, panel, p, serverprotection.Status{}, errors.New("SSH unavailable")); err != nil {
		t.Fatal(err)
	}
	check(false, false) // Keep an actual current verification error visible.
	if err := store.Receipt(ctx, panel, p, protectionReceipt(p), nil); err != nil {
		t.Fatal(err)
	}
	change(3, true)
	check(false, false) // A newer desired revision invalidates the old cleanup.
}
