package kamatera

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"golang.org/x/crypto/ssh"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type credentialFixture string

func (c credentialFixture) Get(context.Context) ([]byte, error) { return []byte(c), nil }

type transportFixture func(*http.Request) (*http.Response, error)

func (f transportFixture) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reply(s string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(s))}
}

const serverID = "11111111-1111-4111-8111-111111111111"
const identity = "dob-deployment-22222222-2222-4222-8222-222222222222"
const credentialJSON = `{"client_id":"fixture-client","secret":"fixture-secret"}`
const sizeJSON = `[{"id":"size1","ramMB":1024,"diskSizeGB":20,"cpuType":"B","cpuCores":1}]`
const imageJSON = `[{"id":"EU:ubuntu-24.04-64bit","name":"Ubuntu 24.04 64bit","code":"ubuntu","datacenter":"EU","osDiskSizeGB":10,"ramMBMin":{"A":1024,"B":1024,"T":1024,"D":1024}}]`

func fixtureDriver(t *testing.T, f transportFixture, j providers.AccountResources) *Driver {
	t.Helper()
	d, e := (Factory{}).Open(context.Background(), providers.OpenRequest{AccountID: "fixture-account", Credentials: credentialFixture(credentialJSON), HTTPClient: &http.Client{Transport: f}, Resources: j})
	if e != nil {
		t.Fatal(e)
	}
	return d.(*Driver)
}
func fixtureKey(t *testing.T) string {
	t.Helper()
	pub, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	key, e := ssh.NewPublicKey(pub)
	if e != nil {
		t.Fatal(e)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
}

type memoryJournal struct {
	mu            sync.Mutex
	record        providers.AccountResource
	operations    int
	regionalError error
}
type memorySession struct{ j *memoryJournal }

func (j *memoryJournal) WithLock(ctx context.Context, scope, name string, f func(providers.AccountResourceSession) error) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.record.Scope == "" {
		j.record.Scope = scope
		j.record.Name = name
	}
	if j.record.Scope != scope || j.record.Name != name {
		return errors.New("intent conflict")
	}
	return f(&memorySession{j})
}

// Production List is an independent read connection; this test journal is used
// sequentially except for the serialized mutation itself.
func (j *memoryJournal) List(context.Context) ([]providers.AccountResource, error) {
	if j.record.Scope == "" {
		return []providers.AccountResource{}, nil
	}
	return []providers.AccountResource{j.record}, nil
}
func (s *memorySession) Record() providers.AccountResource { return s.j.record }
func (s *memorySession) BeginRegionalCreate(ctx context.Context, intent string) (bool, error) {
	if s.j.regionalError != nil {
		return false, s.j.regionalError
	}
	if err := s.PrepareIntent(ctx, intent); err != nil {
		return false, err
	}
	return s.BeginAttempt(ctx)
}
func (s *memorySession) BeginAttempt(context.Context) (bool, error) {
	if s.j.record.Attempted {
		return false, nil
	}
	s.j.record.Attempted = true
	return true, nil
}
func (s *memorySession) Bind(_ context.Context, id string) error {
	if s.j.record.ID != "" && s.j.record.ID != id {
		return errors.New("binding conflict")
	}
	s.j.record.ID = id
	return nil
}
func (s *memorySession) Missing(context.Context) (bool, error) { return false, nil }
func (s *memorySession) Finish(context.Context) error          { s.j.record.Complete = true; return nil }
func (s *memorySession) RecordOperation(_ context.Context, id string) error {
	s.j.record.OperationID = id
	s.j.operations++
	return nil
}
func (s *memorySession) PrepareIntent(_ context.Context, v string) error {
	if s.j.record.Intent != "" && s.j.record.Intent != v {
		return errors.New("intent changed")
	}
	s.j.record.Intent = v
	return nil
}
func (s *memorySession) BeginDeletion(context.Context) (bool, error) {
	if s.j.record.DeleteAttempted {
		return false, nil
	}
	s.j.record.DeleteAttempted = true
	return true, nil
}

func TestKamateraTaskIsNotServerAndLostResponseIsNeverReposted(t *testing.T) {
	for _, empty500 := range []bool{false, true} {
		suffix := ""
		if empty500 {
			suffix = "/empty-500"
		}
		for _, lost := range []bool{false, true} {
			t.Run(map[bool]string{false: "accepted-task", true: "lost-response"}[lost]+suffix, func(t *testing.T) {
				j := &memoryJournal{}
				creates, deletes := 0, 0
				visible := false
				var d *Driver
				d = fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
					if r.Header.Get("AuthClientId") != "fixture-client" || r.Header.Get("AuthSecret") != "fixture-secret" {
						t.Fatal("native authentication missing")
					}
					switch {
					case r.URL.Query().Get("sizes") == "1":
						return reply(sizeJSON), nil
					case r.URL.Query().Get("images") == "1":
						return reply(imageJSON), nil
					case r.URL.Path == "/service/servers":
						if !visible {
							return reply("[]"), nil
						}
						return reply(`[{"id":"` + serverID + `","name":"` + d.intentName(identity) + `","datacenter":"EU","power":"on"}]`), nil
					case r.URL.Path == "/service/server/info":
						if !visible {
							if !empty500 {
								return reply("[]"), nil
							}
							x := reply(emptyWildcardResponse)
							x.StatusCode = 500
							return x, nil
						}
						return reply(`[{"id":"` + serverID + `","name":"` + d.intentName(identity) + `","datacenter":"EU","power":"on","networks":[{"network":"wan","ips":["1.1.1.1"]}]}]`), nil
					case r.URL.Path == "/service/server" && r.Method == "POST":
						creates++
						if !j.record.Attempted || j.record.Intent == "" {
							t.Fatal("dispatch before durable intent")
						}
						var b map[string]any
						json.NewDecoder(r.Body).Decode(&b)
						if b["password"] != "" || b["ssh-key"] == "" || b["quantity"] != float64(1) {
							t.Fatal("invalid native create payload")
						}
						if lost {
							return nil, errors.New("lost response")
						}
						return reply("[12345]"), nil
					case r.URL.Path == "/service/server/terminate":
						deletes++
						return reply("[12346]"), nil
					}
					t.Fatal("unexpected request", r.Method, r.URL.Path)
					return nil, nil
				}, j)
				req := providers.CreateServerRequest{Identity: identity, RegionID: "EU", PlanID: "1B-1024-20", ImageID: "EU:ubuntu-24.04-64bit", SSHAuthorizedKeys: []string{fixtureKey(t)}}
				result, e := d.CreateServer(context.Background(), req)
				if e == nil || result.ServerID != "" || result.Outcome != providers.OutcomeAmbiguous {
					t.Fatal("task mistaken for VM", result, e)
				}
				if !lost && (result.OperationID != "12345" || j.record.OperationID != "12345") {
					t.Fatal("task identity lost")
				}
				for i := 0; i < 3; i++ {
					_, _ = d.FindServerByIdentity(context.Background(), identity)
					_, _ = d.CreateServer(context.Background(), req)
				}
				if creates != 1 {
					t.Fatal("replayed a billable create", creates)
				}
				visible = true
				servers, e := d.FindServerByIdentity(context.Background(), identity)
				if e != nil || len(servers) != 1 || servers[0].ID != serverID || j.record.ID != serverID {
					t.Fatal("durable reconciliation failed", e, servers)
				}
				for i := 0; i < 2; i++ {
					if e = d.DeleteServer(context.Background(), serverID); e != nil {
						t.Fatal(e)
					}
				}
				if deletes != 1 {
					t.Fatal("termination redispatched while unresolved")
				}
				if e = d.DeleteServer(context.Background(), "33333333-3333-4333-8333-333333333333"); e == nil {
					t.Fatal("unowned termination accepted")
				}
			})
		}
	}
}

func TestKamateraCollisionAndRegionMismatchCannotAcquireOwnership(t *testing.T) {
	for _, attempted := range []bool{false, true} {
		j := &memoryJournal{}
		var d *Driver
		d = fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
			return reply(`[{"id":"` + serverID + `","name":"` + d.intentName(identity) + `","datacenter":"US","power":"on"}]`), nil
		}, j)
		j.record = providers.AccountResource{Scope: "server:" + identity, Name: d.intentName(identity), Attempted: attempted, Intent: `{"region":"EU"}`}
		e := j.WithLock(context.Background(), j.record.Scope, j.record.Name, func(s providers.AccountResourceSession) error {
			_, e := d.matchIntent(context.Background(), s)
			return e
		})
		if e == nil || j.record.ID != "" {
			t.Fatal("foreign server adopted")
		}
	}
}

func TestKamateraReadBoundaryAndCredentialScope(t *testing.T) {
	for _, tc := range []struct {
		url, body string
		read      bool
	}{
		{endpoint + "/service/server/info", `{"name":".*"}`, true},
		{endpoint + "/service/server/info", `{"id":"` + serverID + `"}`, true},
		{endpoint + "/service/server/info", `{"name":".*","force":"true"}`, false},
		{endpoint + "/service/server/info?delete=1", `{"name":".*"}`, false},
		{endpoint + "/service/server", `{"name":".*"}`, false},
		{"https://foreign.invalid/service/server/info", `{"name":".*"}`, false},
	} {
		r, _ := http.NewRequest("POST", tc.url, strings.NewReader(tc.body))
		if ReadOnlyRequest(r) != tc.read {
			t.Fatal("read-only request boundary", tc)
		}
	}
	r, _ := http.NewRequest("POST", endpoint+"/service/server/terminate", strings.NewReader(`{"id":"`+serverID+`","force":true}`))
	if TerminationTarget(r) != serverID {
		t.Fatal("exact termination not recognized")
	}
	_, e := (Factory{}).Open(context.Background(), providers.OpenRequest{AccountID: "a", HTTPClient: &http.Client{}, Credentials: credentialFixture(credentialJSON), ExpectedExternalID: CredentialScope("other")})
	if e == nil {
		t.Fatal("credential scope changed silently")
	}
}

func TestKamateraMalformedInventoryAndErrorsDoNotProveAbsence(t *testing.T) {
	for _, raw := range []string{"null", `{"error":"fixture-secret"}`, `[{"id":"12345","name":"x","power":"on","datacenter":"EU"}]`} {
		d := fixtureDriver(t, func(*http.Request) (*http.Response, error) { return reply(raw), nil }, nil)
		_, e := d.Account(context.Background())
		if e == nil || strings.Contains(e.Error(), "fixture-secret") {
			t.Fatal("invalid inventory accepted or secret leaked", e)
		}
	}
	d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			return reply("[]"), nil
		}
		return reply(`[{"id":"` + serverID + `","name":"x","power":"on","datacenter":"EU"}]`), nil
	}, nil)
	_, e := d.GetServer(context.Background(), serverID)
	if e == nil || providers.IsClass(e, providers.ErrorNotFound) {
		t.Fatal("contradictory inventory became absence", e)
	}
}

func TestKamateraCatalogPreservesRegionalImagesAndUnknownQuotaPrice(t *testing.T) {
	d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Host == "www.kamatera.com":
			return reply("unavailable"), nil
		case r.URL.Query().Get("sizes") == "1":
			return reply(sizeJSON), nil
		case r.URL.Query().Get("images") == "1":
			return reply(imageJSON), nil
		case r.URL.Query().Get("capabilities") == "1":
			return reply(`{"diskSizeGB":[20],"monthlyTrafficPackage":{"t5000":"included"},"defaultMonthlyTrafficPackage":"t5000","cpuTypes":[{"id":"B","cpuCores":[1],"ramMB":[1024]}]}`), nil
		case r.URL.Query().Get("datacenter") == "1":
			return reply(`[{"id":"EU","name":"Germany","subCategory":"Frankfurt"}]`), nil
		case r.URL.Path == "/service/servers":
			return reply("[]"), nil
		}
		t.Fatal("unexpected catalog request")
		return nil, nil
	}, nil)
	c, e := d.Catalog(context.Background())
	if e != nil || len(c.Plans) != 1 || len(c.Images) != 1 || c.Images[0].IDsByRegion["EU"] != "EU:ubuntu-24.04-64bit" || c.Pricing.Status != "unavailable" {
		t.Fatal("catalog semantics", c, e)
	}
	cap, e := d.Capacity(context.Background())
	if e != nil || cap.LimitKnown {
		t.Fatal("unknown quota fabricated", cap, e)
	}
}
