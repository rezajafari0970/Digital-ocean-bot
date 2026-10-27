package adminapi

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAccountDeleteStopsPublicationBeforeCleanup(t *testing.T) {
	db := adminTestDB(t)
	f := seedAccountOutput(t, db)
	s := &Server{DB: db}
	token := accountOutputToken(t, s, f[0].Account, "DIRECT")
	other := accountOutputToken(t, s, f[1].Account, "DIRECT")
	start := time.Now()
	w := requestDeletion(s, f[0].Account, auth.Admin)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var receipt struct {
		Removed bool `json:"removed_from_active"`
		Pending bool `json:"cleanup_pending"`
	}
	if json.Unmarshal(w.Body.Bytes(), &receipt) != nil || !receipt.Removed || !receipt.Pending {
		t.Fatal(w.Body.String())
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("deletion request waited for cloud cleanup")
	}
	var retained, stopped bool
	if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM droplets WHERE account_id=$1),NOT enabled AND deletion_requested_at IS NOT NULL FROM accounts WHERE id=$1", f[0].Account).Scan(&retained, &stopped); err != nil || !retained || !stopped {
		t.Fatal("cleanup state lost", err)
	}
	if x := readAccountOutput(s, token, ""); x.Code != 404 {
		t.Fatal("deleted account share remains usable", x.Code)
	}
	if x := readAccountOutput(s, other, ""); x.Code != 200 {
		t.Fatal("other account share damaged", x.Code)
	}
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"route_class":"DIRECT"}`))
	req.SetPathValue("id", f[0].Account)
	next := httptest.NewRecorder()
	s.createAccountOutputShare(next, req)
	if next.Code != 404 {
		t.Fatal("new share created after deletion", next.Code)
	}
	var raw []byte
	if err := db.QueryRowContext(context.Background(), dashboardCountsSQL).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var counts map[string]any
	json.Unmarshal(raw, &counts)
	if counts["digitalocean_accounts"] != float64(0) || counts["vultr_accounts"] != float64(1) {
		t.Fatal("active account counts include deletion", string(raw))
	}
	t.Logf("durable removal request completed in %s; provider cleanup remains separate", time.Since(start))
}
