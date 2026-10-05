package upcloud

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"golang.org/x/crypto/ssh"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const serverID = "00112233-4455-4677-8899-aabbccddeeff"
const storageID = "01112233-4455-4677-8899-aabbccddeeff"
const imageID = "01000000-0000-4000-8000-000030240400"

type credential string

func (c credential) Get(context.Context) ([]byte, error) { return []byte(c), nil }

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type journal struct {
	mu    sync.Mutex
	items map[string]providers.CleanupManifest
	fail  bool
}

func (j *journal) Load(_ context.Context, a, s string) (*providers.CleanupManifest, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	m, ok := j.items[a+s]
	if !ok {
		return nil, nil
	}
	return &m, nil
}
func (j *journal) Save(_ context.Context, a, s string, m providers.CleanupManifest) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.fail {
		return errors.New("database unavailable")
	}
	if j.items == nil {
		j.items = map[string]providers.CleanupManifest{}
	}
	if _, ok := j.items[a+s]; !ok {
		j.items[a+s] = m
	}
	return nil
}
func (j *journal) Finish(_ context.Context, a, s string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	m := j.items[a+s]
	m.Complete = true
	j.items[a+s] = m
	return nil
}
func fixture(t *testing.T, fn func(*http.Request) (int, any, error), j providers.CleanupJournal) *Driver {
	t.Helper()
	h := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.upcloud.com" || r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Errorf("wrong gateway request or credential")
		}
		status, body, e := fn(r)
		if e != nil {
			return nil, e
		}
		b, _ := json.Marshal(body)
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(b))), Request: r}, nil
	})}
	d, e := (Factory{}).Open(context.Background(), providers.OpenRequest{AccountID: "account-A", HTTPClient: h, Credentials: credential("fixture-token"), Cleanup: j, PlanIDs: []string{"1xCPU-1GB"}})
	if e != nil {
		t.Fatal(e)
	}
	return d.(*Driver)
}
func planJSON() any {
	return map[string]any{"plans": map[string]any{"plan": []any{map[string]any{"name": "1xCPU-1GB", "core_number": 1, "memory_amount": 1024, "storage_size": 25, "storage_tier": "maxiops"}, map[string]any{"name": "2xCPU-4GB", "core_number": 2, "memory_amount": 4096, "storage_size": 50, "storage_tier": "maxiops"}}}}
}
func templateJSON() any {
	return map[string]any{"storage": map[string]any{"uuid": imageID, "type": "template", "access": "public", "state": "online", "title": "Ubuntu Server 24.04 LTS", "size": 5}}
}
func key(t *testing.T) string {
	p, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	k, _ := ssh.NewPublicKey(p)
	return string(ssh.MarshalAuthorizedKey(k))
}
func createRequest(t *testing.T) providers.CreateServerRequest {
	return providers.CreateServerRequest{Name: "dob-test", RegionID: "de-fra1", PlanID: "1xCPU-1GB", ImageID: imageID, Identity: "dob-deployment-one", SSHAuthorizedKeys: []string{key(t)}, Tags: []string{"managed-by-digital-ocean-bot"}}
}
func TestCreatePayloadAndAmbiguousAdoption(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint(lost), func(t *testing.T) {
			posts := 0
			var stored serverData
			d := fixture(t, func(r *http.Request) (int, any, error) {
				switch r.URL.Path {
				case "/1.3/plan":
					return 200, planJSON(), nil
				case "/1.3/storage/" + imageID:
					return 200, templateJSON(), nil
				case "/1.3/server":
					if r.Method == "POST" {
						posts++
						var body map[string]map[string]any
						if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
							t.Fatal(e)
						}
						b := body["server"]
						raw, _ := json.Marshal(b)
						if strings.Contains(string(raw), "IPv6") {
							t.Fatal("IPv6 allocated")
						}
						login := b["login_user"].(map[string]any)
						if login["username"] != "root" || login["create_password"] != "no" {
							t.Fatal(login)
						}
						if len(login["ssh_keys"].(map[string]any)["ssh_key"].([]any)) != 1 {
							t.Fatal("missing raw key")
						}
						disks := b["storage_devices"].(map[string]any)["storage_device"].([]any)
						disk := disks[0].(map[string]any)
						if disk["action"] != "clone" || disk["storage"] != imageID || disk["size"] != float64(25) {
							t.Fatal(disk)
						}
						if b["firewall"] != "off" || b["metadata"] != "yes" || b["simple_backup"] != "no" || b["password_delivery"] != "none" {
							t.Fatal("unsafe defaults")
						}
						stored = serverData{ID: serverID, State: "maintenance"}
						lb, _ := json.Marshal(b["labels"])
						json.Unmarshal(lb, &stored.Labels)
						if labelValue(stored.Labels.Items, "dob-owner") != "account-A" || labelValue(stored.Labels.Items, "dob-identity") != "dob-deployment-one" {
							t.Fatal("identity missing")
						}
						if lost {
							return 0, nil, errors.New("response lost after commit")
						}
						return 202, map[string]any{"server": stored}, nil
					}
					ss := []serverData{}
					if r.URL.Query().Get("offset") == "0" {
						ss = append(ss, stored)
					}
					return 200, map[string]any{"servers": map[string]any{"server": ss}}, nil
				}
				t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
				return 500, nil, nil
			}, &journal{})
			got, e := d.CreateServer(context.Background(), createRequest(t))
			if lost {
				if got.Outcome != providers.OutcomeAmbiguous || e == nil {
					t.Fatal(got, e)
				}
			} else if e != nil || got.ServerID != serverID {
				t.Fatal(got, e)
			}
			found, e := d.FindServerByIdentity(context.Background(), "dob-deployment-one")
			if e != nil || len(found) != 1 || found[0].ID != serverID {
				t.Fatal(found, e)
			}
			if posts != 1 {
				t.Fatal("mutation retried", posts)
			}
			other := *d
			other.accountID = "other-account"
			found, e = other.FindServerByIdentity(context.Background(), "dob-deployment-one")
			if e != nil || len(found) != 0 {
				t.Fatal("cross-account adoption")
			}
		})
	}
}
func TestInventoryFailsClosed(t *testing.T) {
	for _, mode := range []string{"missing", "null", "duplicate", "later-page-failure"} {
		t.Run(mode, func(t *testing.T) {
			d := fixture(t, func(r *http.Request) (int, any, error) {
				if mode == "missing" {
					return 200, map[string]any{}, nil
				}
				if mode == "null" {
					return 200, map[string]any{"servers": map[string]any{"server": nil}}, nil
				}
				if mode == "later-page-failure" && r.URL.Query().Get("offset") != "0" {
					return 503, map[string]any{}, nil
				}
				return 200, map[string]any{"servers": map[string]any{"server": []serverData{{ID: serverID}}}}, nil
			}, nil)
			if xs, e := d.rawServers(context.Background()); e == nil || xs != nil {
				t.Fatal("partial inventory returned", xs, e)
			}
		})
	}
}
func TestPaginationAndIPv4Readiness(t *testing.T) {
	d := fixture(t, func(r *http.Request) (int, any, error) {
		if r.URL.Path == "/1.3/ip_address" {
			return 200, map[string]any{"ip_addresses": map[string]any{"ip_address": []ipData{{Address: "203.0.113.12", Family: "IPv4", Access: "public", Server: serverID}, {Address: "2001:db8::1", Family: "IPv6", Access: "public", Server: serverID}}}}, nil
		}
		ss := []serverData{}
		switch r.URL.Query().Get("offset") {
		case "0":
			ss = []serverData{{ID: serverID, State: "started"}}
		case "1":
			ss = []serverData{{ID: "00112233-4455-4677-8899-000000000002", State: "started"}}
		}
		return 200, map[string]any{"servers": map[string]any{"server": ss}}, nil
	}, nil)
	ss, e := d.ListServers(context.Background())
	if e != nil || len(ss) != 2 || !ss[0].Ready || ss[1].Ready {
		t.Fatal(ss, e)
	}
}
func TestCapacityUsesSelectedPlansAndQuotaUnits(t *testing.T) {
	usage := map[string]number{"cores": 4, "memory": 4096, "public_ipv4": 3, "storage_total": 100, "storage_maxiops": 100}
	d := fixture(t, func(r *http.Request) (int, any, error) {
		if r.URL.Path == "/1.3/plan" {
			return 200, planJSON(), nil
		}
		return 200, usage, nil
	}, nil)
	d.planIDs = []string{"1xCPU-1GB", "2xCPU-4GB"}
	var a accountData
	if err := json.Unmarshal([]byte(`{"resource_limits":{"cores":10,"memory":16384,"public_ipv4":10,"storage_total":500,"storage_maxiops":250}}`), &a); err != nil {
		t.Fatal(err)
	}
	cap, e := d.capacity(context.Background(), a, 3)
	if e != nil || !cap.LimitKnown || cap.ComputeLimit != 6 || cap.PlanAvailable["1xCPU-1GB"] != 6 || cap.PlanAvailable["2xCPU-4GB"] != 3 {
		t.Fatal(cap, e)
	}
	*a.Limits["storage_maxiops"] = 100
	cap, e = d.capacity(context.Background(), a, 3)
	if e != nil || cap.ComputeLimit != 3 {
		t.Fatal("zero remaining", cap, e)
	}
	savedCores := a.Limits["cores"]
	delete(a.Limits, "cores")
	if missing, err := d.capacity(context.Background(), a, 3); err == nil || missing.LimitKnown {
		t.Fatal("missing limit accepted", missing, err)
	}
	a.Limits["cores"] = savedCores
	delete(usage, "memory")
	if _, e = d.capacity(context.Background(), a, 3); e == nil {
		t.Fatal("missing usage accepted")
	}
}
func TestDeleteDurableRecoveryAndOwnedStorageOnly(t *testing.T) {
	j := &journal{}
	state := "started"
	serverGone := false
	diskGone := false
	lostDelete := true
	stops := 0
	deletes := 0
	labels := []label{{"dob-owner", "account-A"}, {"dob-identity", "dob-deployment-one"}}
	makeDriver := func() *Driver {
		return fixture(t, func(r *http.Request) (int, any, error) {
			if r.Method == "GET" && r.URL.Path == "/1.3/server/"+serverID {
				if serverGone {
					return 404, map[string]any{}, nil
				}
				s := serverData{ID: serverID, State: state}
				s.Labels.Items = labels
				raw, _ := json.Marshal(map[string]any{"storage_device": []any{map[string]any{"storage": storageID, "type": "disk"}}})
				json.Unmarshal(raw, &s.Disks)
				return 200, map[string]any{"server": s}, nil
			}
			if r.URL.Path == "/1.3/storage/private" {
				return 200, map[string]any{"storages": map[string]any{"storage": []storageData{{ID: storageID, Type: "normal", State: "online", Labels: labels}}}}, nil
			}
			if r.URL.Path == "/1.3/storage/"+storageID {
				if r.Method == "DELETE" {
					diskGone = true
					return 204, nil, nil
				}
				if diskGone {
					return 404, map[string]any{}, nil
				}
				s := storageData{ID: storageID, State: "online", Labels: labels}
				return 200, map[string]any{"storage": s}, nil
			}
			if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/stop") {
				m, _ := j.Load(context.Background(), "account-A", serverID)
				if m == nil {
					t.Fatal("stop before journal")
				}
				stops++
				state = "stopped"
				return 200, map[string]any{}, nil
			}
			if r.Method == "DELETE" && r.URL.Path == "/1.3/server/"+serverID {
				if r.URL.Query().Get("storages") != "0" {
					t.Fatal("broad storage deletion")
				}
				deletes++
				serverGone = true
				if lostDelete {
					lostDelete = false
					return 0, nil, errors.New("lost delete response")
				}
				return 204, nil, nil
			}
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			return 500, nil, nil
		}, j)
	}
	d := makeDriver()
	if e := d.DeleteServer(context.Background(), serverID); !providers.IsRetryable(e) {
		t.Fatal("asynchronous stop", e)
	}
	if e := d.DeleteServer(context.Background(), serverID); !providers.IsRetryable(e) {
		t.Fatal("lost response", e)
	}
	d = makeDriver() // Reconstruct process-local state using only the persisted journal.
	s, e := d.GetServer(context.Background(), serverID)
	if e != nil || s.State != providers.ServerStateDeleting {
		t.Fatal("premature absence", s, e)
	}
	if e = d.DeleteServer(context.Background(), serverID); e != nil {
		t.Fatal(e)
	}
	if _, e = d.GetServer(context.Background(), serverID); !providers.IsClass(e, providers.ErrorNotFound) {
		t.Fatal(e)
	}
	if !diskGone || stops != 1 || deletes != 1 {
		t.Fatal(diskGone, stops, deletes)
	}
	if e = d.DeleteServer(context.Background(), serverID); e != nil {
		t.Fatal("idempotent replay", e)
	}
}
func TestDeleteJournalFailureBlocksRemoteMutation(t *testing.T) {
	count := 0
	d := fixture(t, func(r *http.Request) (int, any, error) {
		if r.Method != "GET" {
			count++
		}
		if r.URL.Path == "/1.3/storage/private" {
			return 200, map[string]any{"storages": map[string]any{"storage": []any{}}}, nil
		}
		s := serverData{ID: serverID, State: "started"}
		s.Labels.Items = []label{{"dob-owner", "account-A"}, {"dob-identity", "identity"}}
		return 200, map[string]any{"server": s}, nil
	}, &journal{fail: true})
	if e := d.DeleteServer(context.Background(), serverID); e == nil || count != 0 {
		t.Fatal(e, count)
	}
}
func TestErrorClassificationAndRedaction(t *testing.T) {
	cases := []struct {
		status int
		code   string
		want   providers.ErrorClass
	}{{401, "AUTHENTICATION_FAILED", providers.ErrorAuthentication}, {402, "INSUFFICIENT_CREDITS", providers.ErrorBilling}, {403, "SERVER_CORE_LIMIT_REACHED", providers.ErrorCapacity}, {403, "SERVER_FORBIDDEN", providers.ErrorPermissionDenied}, {409, "SERVER_RESOURCES_UNAVAILABLE", providers.ErrorRegionCapacity}, {409, "SERVER_STATE_ILLEGAL", providers.ErrorUnavailable}, {429, "RATE_LIMITED", providers.ErrorRateLimited}, {503, "SERVICE_UNAVAILABLE", providers.ErrorUnavailable}}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			d := fixture(t, func(*http.Request) (int, any, error) {
				return tc.status, map[string]any{"error": map[string]any{"error_code": tc.code, "error_message": "fixture-token"}}, nil
			}, nil)
			_, e := d.Account(context.Background())
			if providers.Class(e) != tc.want || strings.Contains(e.Error(), "fixture-token") {
				t.Fatal(e)
			}
		})
	}
}
func TestTransportRedirectRefused(t *testing.T) {
	hits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; w.WriteHeader(200) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer origin.Close()
	c := newClient(origin.Client(), credential("fixture-token"))
	c.base = origin.URL
	e := c.do(context.Background(), "GET", "/account", nil, &map[string]any{})
	if e == nil || hits != 0 {
		t.Fatal("redirect followed", e, hits)
	}
}
func TestMalformedCreateNeverBecomesRejectedRetry(t *testing.T) {
	posts := 0
	d := fixture(t, func(r *http.Request) (int, any, error) {
		if r.URL.Path == "/1.3/plan" {
			return 200, planJSON(), nil
		}
		if r.URL.Path == "/1.3/storage/"+imageID {
			return 200, templateJSON(), nil
		}
		posts++
		return 202, map[string]any{"server": map[string]any{}}, nil
	}, nil)
	out, e := d.CreateServer(context.Background(), createRequest(t))
	if e == nil || out.Outcome != providers.OutcomeAmbiguous || posts != 1 {
		t.Fatal(out, e, posts)
	}
}
func TestFactoryMetadataAndBilling(t *testing.T) {
	d := fixture(t, func(r *http.Request) (int, any, error) {
		if r.URL.Path == "/1.3/account" {
			return 200, map[string]any{"account": map[string]any{"username": "fixture-user", "credits": 12.25}}, nil
		}
		return 200, map[string]any{"currency": "EUR", "total_amount": 1.5}, nil
	}, nil)
	if d.Name() != "upcloud" || !d.Capabilities().InlineSSHKeys || d.Capabilities().SSHKeys {
		t.Fatal("incorrect key capability")
	}
	b, e := d.Billing(context.Background())
	if e != nil || b.Balance != "12.25" || b.Currency != "EUR" || b.PendingCharges != "" || b.MonthToDate != "1.5" || time.Since(b.GeneratedAt) > time.Minute {
		t.Fatal(b, e)
	}
}
