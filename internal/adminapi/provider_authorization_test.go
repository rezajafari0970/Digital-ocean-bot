package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ovh"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const authorizationFixtureID = "11111111-1111-4111-8111-111111111111"
const authorizationFixtureProject = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const authorizationFixtureToken = `{"endpoint":"ovh-eu","application_key":"fixture-app","application_secret":"fixture-secret","consumer_key":"fixture-consumer"}`

type authorizationFactoryFixture struct {
	ovh.Factory
	calls   atomic.Int32
	checks  atomic.Int32
	unknown bool
}

func (f *authorizationFactoryFixture) StartAuthorization(ctx context.Context, r providers.OpenRequest) (providers.AuthorizationGrant, error) {
	f.calls.Add(1)
	time.Sleep(20 * time.Millisecond)
	if f.unknown {
		return providers.AuthorizationGrant{}, &providers.Error{Class: providers.ErrorAmbiguousOutcome, Code: "AUTHORIZATION_URL_UNKNOWN"}
	}
	return providers.AuthorizationGrant{Credentials: previewCredential{[]byte(authorizationFixtureToken)}, ValidationURL: "https://eu.api.ovh.com/auth/?credentialToken=fixture-validation"}, nil
}
func (f *authorizationFactoryFixture) CheckAuthorization(context.Context, providers.OpenRequest) (bool, error) {
	f.checks.Add(1)
	return true, nil
}
func (f *authorizationFactoryFixture) Discover(ctx context.Context, r providers.OpenRequest, selection map[string]string) (providers.Discovery, error) {
	var cred map[string]any
	b, _ := r.Credentials.Get(ctx)
	json.Unmarshal(b, &cred)
	cred["project_id"] = authorizationFixtureProject
	b, _ = json.Marshal(cred)
	return providers.Discovery{Complete: true, Resolved: map[string]any{"endpoint": "ovh-eu", "project_id": authorizationFixtureProject}, Credentials: previewCredential{b}}, nil
}
func (f *authorizationFactoryFixture) Open(context.Context, providers.OpenRequest) (providers.Driver, error) {
	return authorizationDriverFixture{}, nil
}

type authorizationDriverFixture struct{}

func (authorizationDriverFixture) Name() string { return "ovh" }
func (authorizationDriverFixture) Capabilities() providers.Capabilities {
	return providers.Capabilities{Account: true, Catalog: true}
}
func (authorizationDriverFixture) Health(context.Context) error { return nil }
func (authorizationDriverFixture) Account(context.Context) (providers.Account, error) {
	return providers.Account{ID: "ovh-eu:" + authorizationFixtureProject}, nil
}
func (authorizationDriverFixture) Capacity(context.Context) (providers.Capacity, error) {
	return providers.Capacity{ComputeLimit: 5, LimitKnown: true}, nil
}
func (authorizationDriverFixture) Catalog(context.Context) (providers.Catalog, error) {
	return providers.Catalog{Regions: []providers.Region{{ID: "GRA11", Available: true}}, Plans: []providers.Plan{{ID: "b3-8", Available: true}}, Images: []providers.Image{{ID: "ubuntu", Available: true}}}, nil
}
func authorizationTestServer(t *testing.T) (*Server, *authorizationFactoryFixture) {
	t.Helper()
	db := adminTestDB(t)
	store, e := secrets.NewStore(secrets.SQLRepository{DB: db}, bytes.Repeat([]byte{1}, 32), 1)
	if e != nil {
		t.Fatal(e)
	}
	f := &authorizationFactoryFixture{}
	reg := providers.NewRegistry()
	if e = reg.Register(f); e != nil {
		t.Fatal(e)
	}
	return &Server{DB: db, Container: app.Container{Providers: reg, Secrets: store}}, f
}
func authorizationRequest(s *Server, owner, method, path string, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{UserID: owner, Role: auth.Admin}))
	w := httptest.NewRecorder()
	switch {
	case path == "/start":
		r.SetPathValue("provider", "ovh")
		s.startProviderAuthorization(w, r)
	case path == "/preview":
		s.accountPreview(w, r)
	case path == "/save":
		s.createAccount(w, r)
	default:
		r.SetPathValue("id", strings.TrimPrefix(path, "/"))
		if method == "DELETE" {
			s.cancelProviderAuthorization(w, r)
		} else {
			s.providerAuthorizationStatus(w, r)
		}
	}
	return w
}
func authorizationStartBody(id string) map[string]string {
	return map[string]string{"id": id, "token": `{"application_key":"fixture-app","application_secret":"fixture-secret"}`}
}
func assertAuthorizationNoSecret(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	for _, x := range []string{"fixture-consumer", "fixture-secret", "prepared_token"} {
		if strings.Contains(w.Body.String(), x) {
			t.Fatal("secret exposed in API")
		}
	}
}
func TestProviderAuthorizationJournalConcurrencyRestartAndOwnership(t *testing.T) {
	s, f := authorizationTestServer(t)
	var wg sync.WaitGroup
	results := make(chan *httptest.ResponseRecorder, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- authorizationRequest(s, "owner", "POST", "/start", authorizationStartBody(authorizationFixtureID))
		}()
	}
	wg.Wait()
	close(results)
	for w := range results {
		if w.Code != 200 && w.Code != 409 {
			t.Fatal(w.Code, w.Body.String())
		}
		assertAuthorizationNoSecret(t, w)
	}
	if f.calls.Load() != 1 {
		t.Fatal("duplicate credential dispatch", f.calls.Load())
	}
	restarted := &Server{DB: s.DB, Container: s.Container}
	w := authorizationRequest(restarted, "owner", "POST", "/start", authorizationStartBody(authorizationFixtureID))
	if w.Code != 200 || f.calls.Load() != 1 {
		t.Fatal("restart replayed", w.Body.String())
	}
	w = authorizationRequest(restarted, "other", "GET", "/"+authorizationFixtureID, nil)
	if w.Code != 404 {
		t.Fatal("cross-owner read allowed", w.Code)
	}
	assertAuthorizationNoSecret(t, w)
	w = authorizationRequest(restarted, "owner", "GET", "/"+authorizationFixtureID, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"READY"`) {
		t.Fatal(w.Body.String())
	}
	w = authorizationRequest(restarted, "owner", "POST", "/preview", map[string]any{"provider": "ovh", "authorization_id": authorizationFixtureID, "selection": map[string]string{"endpoint": "ovh-us"}})
	if w.Code != 409 {
		t.Fatal("changed scope allowed", w.Code)
	}
	w = authorizationRequest(restarted, "owner", "POST", "/preview", map[string]string{"provider": "ovh", "authorization_id": authorizationFixtureID})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	assertAuthorizationNoSecret(t, w)
	x, e := s.loadAuthorization(context.Background(), "owner", authorizationFixtureID)
	if e != nil || x.State != "VERIFIED" || bytes.Contains(x.Cipher.Ciphertext, []byte("fixture-consumer")) {
		t.Fatal("secret not durable encrypted", e)
	}
	ctx := context.WithValue(context.Background(), principalKey{}, auth.Principal{UserID: "owner", Role: auth.Admin})
	_, data, e := restarted.authorizationCredential(ctx, authorizationFixtureID, "ovh", "", true)
	if e != nil || !strings.Contains(data.Token, authorizationFixtureProject) {
		t.Fatal("prepared selection not durable", e)
	}
	unlock, ok, e := s.tryAuthorizationLock(ctx, authorizationFixtureID)
	if e != nil || !ok {
		t.Fatal(e)
	}
	w = authorizationRequest(s, "owner", "DELETE", "/"+authorizationFixtureID, nil)
	if w.Code != 409 {
		t.Fatal("cancel raced active operation", w.Code)
	}
	unlock()
	w = authorizationRequest(s, "owner", "DELETE", "/"+authorizationFixtureID, nil)
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	x, _ = s.loadAuthorization(ctx, "owner", authorizationFixtureID)
	if x.State != "CANCELLED" || len(x.Cipher.Ciphertext) != 0 {
		t.Fatal("cancel retained credential")
	}
}
func TestProviderAuthorizationUnknownExpiryAndRetainedKeys(t *testing.T) {
	s, f := authorizationTestServer(t)
	f.unknown = true
	w := authorizationRequest(s, "owner", "POST", "/start", authorizationStartBody(authorizationFixtureID))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"UNKNOWN"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for i := 0; i < 3; i++ {
		authorizationRequest(s, "owner", "POST", "/start", authorizationStartBody(authorizationFixtureID))
	}
	if f.calls.Load() != 1 {
		t.Fatal("unknown outcome redispatched")
	}
	f.unknown = false
	id := "22222222-2222-4222-8222-222222222222"
	w = authorizationRequest(s, "owner", "POST", "/start", authorizationStartBody(id))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, e := secrets.NewStore(secrets.SQLRepository{DB: s.DB}, bytes.Repeat([]byte{2}, 32), 2); e == nil {
		t.Fatal("pending authorization key retired")
	}
	if _, e := secrets.NewStoreWithKeys(secrets.SQLRepository{DB: s.DB}, bytes.Repeat([]byte{2}, 32), 2, map[int][]byte{1: bytes.Repeat([]byte{1}, 32)}); e != nil {
		t.Fatal("retained key rejected", e)
	}
	sqlMust(t, s.DB, "UPDATE provider_authorizations SET expires_at=now()-interval '1 second' WHERE id=$1", id)
	unlock, ok, e := s.tryAuthorizationLock(context.Background(), id)
	if e != nil || !ok {
		t.Fatal(e)
	}
	if e = s.CleanupProviderAuthorizations(context.Background()); e != nil {
		t.Fatal(e)
	}
	x, _ := s.loadAuthorization(context.Background(), "owner", id)
	if len(x.Cipher.Ciphertext) == 0 {
		t.Fatal("cleanup raced active save")
	}
	unlock()
	if e = s.CleanupProviderAuthorizations(context.Background()); e != nil {
		t.Fatal(e)
	}
	x, _ = s.loadAuthorization(context.Background(), "owner", id)
	if x.State != "EXPIRED" || len(x.Cipher.Ciphertext) != 0 {
		t.Fatal("expired secret retained")
	}
	if _, e := secrets.NewStore(secrets.SQLRepository{DB: s.DB}, bytes.Repeat([]byte{2}, 32), 2); e != nil {
		t.Fatal("expired key blocked rotation", e)
	}
	sqlMust(t, s.DB, "UPDATE provider_authorizations SET state='DISPATCHED',updated_at=now()-interval '1 minute',expires_at=now()+interval '20 minutes' WHERE id=$1", authorizationFixtureID)
	if e = s.CleanupProviderAuthorizations(context.Background()); e != nil {
		t.Fatal(e)
	}
	x, _ = s.loadAuthorization(context.Background(), "owner", authorizationFixtureID)
	if x.State != "UNKNOWN" {
		t.Fatal("crashed dispatch not reconciled")
	}
}
func TestProviderAuthorizationAtomicAccountHandoff(t *testing.T) {
	s, _ := authorizationTestServer(t)
	authorizationRequest(s, "owner", "POST", "/start", authorizationStartBody(authorizationFixtureID))
	authorizationRequest(s, "owner", "GET", "/"+authorizationFixtureID, nil)
	w := authorizationRequest(s, "owner", "POST", "/preview", map[string]string{"provider": "ovh", "authorization_id": authorizationFixtureID})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	body := accountWrite{AuthorizationID: authorizationFixtureID, Provider: "ovh", Name: "fixture", ExternalID: "ovh-eu:" + authorizationFixtureProject, Regions: []string{"GRA11"}, Sizes: []string{"b3-8"}, Images: []string{"ubuntu"}, NetworkMode: "direct", LifetimeMinSeconds: 1800, LifetimeMaxSeconds: 3600, BuildSpacingMinutes: 60, BuildSpacingMaxMinutes: 120, DesiredServerCount: 1}
	// Failure late in account creation (missing installer) must roll back every row.
	sqlMust(t, s.DB, "UPDATE installers SET active=false")
	w = authorizationRequest(s, "owner", "POST", "/save", body)
	if w.Code != 500 {
		t.Fatal("expected automation failure", w.Code, w.Body.String())
	}
	var count int
	s.DB.QueryRow("SELECT count(*) FROM accounts WHERE provider='ovh'").Scan(&count)
	if count != 0 {
		t.Fatal("partial account survived rollback")
	}
	x, _ := s.loadAuthorization(context.Background(), "owner", authorizationFixtureID)
	if x.State != "VERIFIED" || len(x.Cipher.Ciphertext) == 0 {
		t.Fatal("rollback consumed authorization")
	}
	sqlMust(t, s.DB, `INSERT INTO installers(name,version,manifest,sha256,active) VALUES('authorization-fixture',1,'{"capabilities":["xui_database","xui_panel"]}',repeat('a',64),true)`)
	w = authorizationRequest(s, "owner", "POST", "/save", body)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	assertAuthorizationNoSecret(t, w)
	x, _ = s.loadAuthorization(context.Background(), "owner", authorizationFixtureID)
	if x.State != "USED" || x.AccountID == "" || len(x.Cipher.Ciphertext) != 0 {
		t.Fatal("receipt/secret transfer incomplete")
	}
	secret, e := s.Container.Secrets.Get(context.Background(), x.AccountID, "provider-primary")
	if e != nil || !bytes.Contains(secret, []byte("fixture-consumer")) || !bytes.Contains(secret, []byte(authorizationFixtureProject)) {
		t.Fatal("account credential not transferred", e)
	}
	w = authorizationRequest(s, "owner", "POST", "/save", body)
	if w.Code != 200 || !strings.Contains(w.Body.String(), x.AccountID) {
		t.Fatal("lost response recovery failed", w.Code, w.Body.String())
	}
	s.DB.QueryRow("SELECT count(*) FROM accounts WHERE provider='ovh'").Scan(&count)
	if count != 1 {
		t.Fatal("duplicate account")
	}
}

func TestProviderAuthorizationSafeDiagnosticSurvivesRestart(t *testing.T) {
	s, f := authorizationTestServer(t)
	f.unknown = true
	w := authorizationRequest(s, "owner", "POST", "/start", authorizationStartBody(authorizationFixtureID))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	restarted := &Server{DB: s.DB, Container: s.Container}
	w = authorizationRequest(restarted, "owner", "GET", "/"+authorizationFixtureID, nil)
	var out struct{ State, Diagnostic, Detail string }
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || out.State != "UNKNOWN" || out.Diagnostic != "AUTHORIZATION_URL_UNKNOWN" || !strings.Contains(out.Detail, "approval URL") || f.calls.Load() != 1 {
		t.Fatalf("safe reason lost or dispatch replayed: %s", w.Body.String())
	}
	assertAuthorizationNoSecret(t, w)
}
