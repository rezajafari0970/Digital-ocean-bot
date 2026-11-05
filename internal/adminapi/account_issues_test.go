package adminapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
)

func TestAccountDiagnosticDetail(t *testing.T) {
	for _, input := range []string{
		`{"provider_error":"provider denied: token=private-canary socks5://user:private-pass@proxy.invalid","secret":"hidden-canary"}`,
		`provider denied: token=private-canary socks5://user:private-pass@proxy.invalid`,
		`{"secret":"hidden-canary"}`, `{"broken":"private-canary`,
	} {
		got := accountDiagnosticDetail(input, "Observation failed")
		if got == "" || strings.Contains(got, "canary") || strings.Contains(got, "private-pass") || strings.Contains(got, "proxy.invalid") || strings.Contains(got, "{") {
			t.Fatalf("unsafe diagnostic: %q", got)
		}
	}
	if got := accountDiagnosticDetail(strings.Repeat("خطا", 500), ""); len([]rune(got)) != 801 {
		t.Fatal("unbounded or broken Unicode detail")
	}
}

func TestAccountIssuesAllProviderParityPostgres(t *testing.T) {
	db := adminTestDB(t)
	s := Server{DB: db}
	for _, provider := range []string{"digitalocean", "vultr", "linode", "aws", "kamatera", "upcloud", "ovh"} {
		id := seedBuildAccount(t, db, provider)
		putBuildSnapshot(t, db, id, "active", true, 10, 1)
		for _, state := range []string{"LOCKED", "TOKEN_INVALID", "PERMISSION_DENIED", "BILLING_BLOCKED"} {
			sqlMust(t, db, `UPDATE accounts SET provider_state=$2,provider_state_detail='provider rejected this operation',provider_state_at=now(),provider_error_state='PROXY_ERROR',provider_error_detail='{"provider_error":"proxy route not verified","secret":"hidden-canary"}',provider_error_at=now() WHERE id=$1`, id, state)
			b, err := s.accountBuildState(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			if b.ProviderCanCreate || b.SchedulerCanBuild || len(b.Issues) != 2 || !strings.Contains(b.Reason, "provider rejected this operation") {
				t.Fatalf("%s %s: %+v", provider, state, b)
			}
			if b.Issues[0].Code != state || b.Issues[1].Code != "PROXY_ERROR" || b.Issues[1].Detail != "proxy route not verified" || b.Issues[0].ObservedAt == nil || b.Issues[1].ObservedAt == nil {
				t.Fatalf("independent evidence lost: %+v", b.Issues)
			}
		}
		list := httptest.NewRecorder()
		s.accounts(list, httptest.NewRequest("GET", "/", nil))
		if list.Code != 200 {
			t.Fatal(list.Code, list.Body.String())
		}
		var rows []map[string]any
		if err := json.Unmarshal(list.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		var row map[string]any
		for _, r := range rows {
			if r["id"] == id {
				row = r
			}
		}
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.SetPathValue("id", id)
		s.accountDashboard(rr, req)
		if rr.Code != 200 {
			t.Fatal(rr.Code, rr.Body.String())
		}
		var dashboard accountDashboard
		if err := json.Unmarshal(rr.Body.Bytes(), &dashboard); err != nil {
			t.Fatal(err)
		}
		a, _ := json.Marshal(row["issues"])
		b, _ := json.Marshal(dashboard.Account["issues"])
		if string(a) != string(b) || strings.Contains(string(a), "hidden-canary") {
			t.Fatal("list/detail diagnostics differ or expose metadata")
		}
		// Disabled display and a satisfied target cannot suppress retained errors
		// or ambiguous provider dispatches, even with no saturated schedule.
		sqlMust(t, db, `UPDATE accounts SET enabled=false,desired_server_count=1 WHERE id=$1`, id)
		sqlMust(t, db, `INSERT INTO operations(id,account_id,kind,idempotency_key,state,attempt) VALUES(gen_random_uuid(),$1,'CREATE_DROPLET','unresolved-issue','unknown',1)`, id)
		status, err := s.accountBuildState(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if status.State != "DISABLED" || len(status.Issues) != 3 || status.Issues[2].Code != "CREATE_OUTCOME_UNKNOWN" {
			t.Fatalf("hidden unresolved create: %+v", status)
		}
		var attempt int
		var operationState string
		if err = db.QueryRow(`SELECT attempt,state FROM operations WHERE account_id=$1`, id).Scan(&attempt, &operationState); err != nil {
			t.Fatal(err)
		}
		if attempt != 1 || operationState != "unknown" {
			t.Fatal("diagnostic read mutated create evidence")
		}
		sqlMust(t, db, `UPDATE accounts SET enabled=true,provider_state='ACTIVE',provider_error_state=NULL,provider_error_detail=NULL WHERE id=$1`, id)
		sqlMust(t, db, `DELETE FROM operations WHERE account_id=$1`, id)
		status, err = s.accountBuildState(context.Background(), id)
		if err != nil || len(status.Issues) != 0 {
			t.Fatalf("resolved evidence still displayed: %+v %v", status, err)
		}
	}
}

func TestAccountIssuesJavaScript(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node unavailable")
	}
	out, err := exec.Command("node", "../../web/tests/account-issues.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestAccountRefreshRuntimeFailureAndPreflightIssuesPostgres(t *testing.T) {
	db := adminTestDB(t)
	id := seedBuildAccount(t, db, "upcloud")
	putBuildSnapshot(t, db, id, "active", true, 10, 0)
	sqlMust(t, db, `UPDATE accounts SET provider_state='LOCKED',provider_state_detail='retained provider restriction' WHERE id=$1`, id)
	sqlMust(t, db, `UPDATE network_profiles SET mode='proxy_required' WHERE account_id=$1`, id)
	s := Server{DB: db, Container: app.Container{DB: db}}
	req := httptest.NewRequest("POST", "/", nil)
	req.SetPathValue("id", id)
	rr := httptest.NewRecorder()
	s.accountDiscovery(rr, req)
	if rr.Code != 502 || !strings.Contains(rr.Body.String(), "PROXY_ERROR") {
		t.Fatal(rr.Code, rr.Body.String())
	}
	status, err := s.accountBuildState(context.Background(), id)
	if err != nil || len(status.Issues) != 2 || status.Issues[0].Code != "LOCKED" || status.Issues[1].Code != "PROXY_ERROR" {
		t.Fatalf("runtime failure not recorded independently: %+v %v", status, err)
	}
	if err = capacity.RecordCreateBlock(context.Background(), db, id, trialDenied()); err != nil {
		t.Fatal(err)
	}
	req = req.WithContext(context.WithValue(req.Context(), principalKey{}, auth.Principal{Role: auth.Admin}))
	rr = httptest.NewRecorder()
	s.accountPreflight(rr, req)
	var response struct {
		Error         string          `json:"error"`
		ProviderError string          `json:"provider_error"`
		CreateError   string          `json:"create_error"`
		Checks        map[string]bool `json:"checks"`
	}
	if err = json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if rr.Code != 409 || response.Error != "account_preflight_failed" || !strings.Contains(response.ProviderError, "network") || !strings.Contains(response.CreateError, "TRIAL_FIREWALL") || response.Checks["create_permission"] || response.Checks["provider_identity"] {
		t.Fatal(rr.Code, rr.Body.String())
	}
	status, err = s.accountBuildState(context.Background(), id)
	if err != nil || len(status.Issues) != 3 || !strings.Contains(status.Issues[2].Detail, "TRIAL_FIREWALL") {
		t.Fatalf("create block replaced by provider restriction: %+v %v", status, err)
	}
}