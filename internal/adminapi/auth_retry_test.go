package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"net/http/httptest"
	"strings"
	"testing"
)

type repairCredentialFactory struct{}

func (repairCredentialFactory) Name() string { return "digitalocean" }
func (repairCredentialFactory) Metadata() providers.Metadata {
	return providers.Metadata{Name: "digitalocean", Status: "ready"}
}
func (repairCredentialFactory) Open(ctx context.Context, r providers.OpenRequest) (providers.Driver, error) {
	token, err := r.Credentials.Get(ctx)
	return repairCredentialDriver{valid: string(token) == "replacement"}, err
}

type repairCredentialDriver struct{ valid bool }

var _ providers.AccountReader = repairCredentialDriver{}

func (repairCredentialDriver) Name() string                 { return "digitalocean" }
func (repairCredentialDriver) Health(context.Context) error { return nil }
func (repairCredentialDriver) Capabilities() providers.Capabilities {
	return providers.Capabilities{Account: true}
}
func (repairCredentialDriver) Capacity(context.Context) (providers.Capacity, error) {
	return providers.Capacity{}, nil
}
func (d repairCredentialDriver) Account(context.Context) (providers.Account, error) {
	if !d.valid {
		return providers.Account{}, &providers.Error{Class: providers.ErrorAuthentication, StatusCode: 401}
	}
	return providers.Account{ID: "fixture-account", Status: "active"}, nil
}

func TestValidatedCredentialReplacementRearmsAuthRetriesPostgres(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	a, _ := ruleFixture(t, db, "digitalocean", 1)
	other := seedBuildAccount(t, db, "digitalocean")
	store, err := secrets.NewStore(secrets.SQLRepository{DB: db}, bytes.Repeat([]byte{7}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Put(ctx, a, "fixture", "digitalocean_credential", []byte("original")); err != nil {
		t.Fatal(err)
	}
	registry := providers.NewRegistry()
	if err = registry.Register(repairCredentialFactory{}); err != nil {
		t.Fatal(err)
	}
	s := Server{DB: db, Container: app.Container{DB: db, Secrets: store, Providers: registry}}
	for _, v := range []struct{ account, kind, item, marker string }{{a, "operation", "auth", worker.ProviderAuthRetryMarker}, {a, "lifecycle", "similar", worker.ProviderAuthRetryMarker + "_extra"}, {a, "deployment", "wrong-kind", worker.ProviderAuthRetryMarker}, {other, "operation", "other", worker.ProviderAuthRetryMarker}} {
		sqlMust(t, db, `INSERT INTO worker_item_failures(kind,item_id,account_id,failures,last_error,first_failed_at,last_failed_at,next_retry_at) VALUES($1,$2,$3,17,$4,now(),now(),now()+interval '5 minutes')`, v.kind, v.item, v.account, v.marker)
	}
	op := perfUUID()
	sqlMust(t, db, `INSERT INTO operations(id,account_id,kind,idempotency_key,state) VALUES($1,$2,'DELETE_DROPLET',$3,'unknown')`, op, a, op)
	sqlMust(t, db, `INSERT INTO worker_recovery_checkpoints(kind,item_id,account_id) VALUES('operation',$1,$2)`, op, a)
	save := func(token string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"name": "repair fixture", "token": token, "regions": []string{"ams"}, "sizes": []string{"small"}, "images": []string{"ubuntu-24-04-x64"}, "image": "ubuntu-24-04-x64", "network_mode": "direct", "lifetime_min_seconds": 3600, "lifetime_max_seconds": 5400, "build_spacing_minutes": 1, "build_spacing_max_minutes": 3, "desired_server_count": 1, "max_concurrent": 3, "fallback_any_region": true})
		r := httptest.NewRequest("PUT", "/", strings.NewReader(string(body)))
		r.SetPathValue("id", a)
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin}))
		w := httptest.NewRecorder()
		s.updateAccount(w, r)
		return w
	}
	ledger := worker.FailureStore{DB: db}
	if w := save("invalid"); w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
	if ledger.Due(ctx, "operation", "auth") {
		t.Fatal("invalid token rearmed")
	}
	sqlMust(t, db, `CREATE FUNCTION reject_auth_rearm() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.next_retry_at IS DISTINCT FROM OLD.next_retry_at THEN RAISE EXCEPTION 'fixture fault'; END IF; RETURN NEW; END$$;
CREATE TRIGGER reject_auth_rearm BEFORE UPDATE ON worker_item_failures FOR EACH ROW EXECUTE FUNCTION reject_auth_rearm()`)
	if w := save("replacement"); w.Code != 500 {
		t.Fatal(w.Code, w.Body.String())
	}
	if ledger.Due(ctx, "operation", "auth") {
		t.Fatal("rolled-back save rearmed")
	}
	old, err := store.Get(ctx, a, "fixture")
	if err != nil || string(old) != "original" {
		t.Fatal("failed save changed credential", err)
	}
	sqlMust(t, db, `DROP TRIGGER reject_auth_rearm ON worker_item_failures`)
	if w := save("replacement"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !ledger.Due(ctx, "operation", "auth") {
		t.Fatal("valid replacement did not rearm")
	}
	for _, v := range []struct{ kind, item string }{{"lifecycle", "similar"}, {"deployment", "wrong-kind"}, {"operation", "other"}} {
		if ledger.Due(ctx, v.kind, v.item) {
			t.Fatal("unrelated failure rearmed", v)
		}
	}
	var failures, checkpoints int
	var state string
	if err = db.QueryRow(`SELECT failures FROM worker_item_failures WHERE item_id='auth'`).Scan(&failures); err != nil || failures != 17 {
		t.Fatal(failures, err)
	}
	if err = db.QueryRow(`SELECT state FROM operations WHERE id=$1`, op).Scan(&state); err != nil || state != "unknown" {
		t.Fatal(state, err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM worker_recovery_checkpoints WHERE account_id=$1`, a).Scan(&checkpoints); err != nil || checkpoints != 1 {
		t.Fatal(checkpoints, err)
	}
}
