package linode

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"golang.org/x/crypto/ssh"
)

type credential struct{}

func (credential) Get(context.Context) ([]byte, error) { return []byte("fixture-token"), nil }

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func driver(t *testing.T, handler roundTrip) *Driver {
	t.Helper()
	d, e := (Factory{}).Open(context.Background(), providers.OpenRequest{AccountID: "11111111-1111-4111-8111-111111111111", HTTPClient: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "api.linode.com" || r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Fatalf("unsafe request %s", r.URL)
		}
		if r.URL.Path == "/v4/profile" {
			return response(200, `{"restricted":false}`), nil
		}
		return handler(r)
	})}, Credentials: credential{}})
	if e != nil {
		t.Fatal(e)
	}
	return d.(*Driver)
}
func page(data string, number, pages, total int) string {
	return fmt.Sprintf(`{"data":%s,"page":%d,"pages":%d,"results":%d}`, data, number, pages, total)
}
func server(n int, tags []string) string {
	raw, _ := json.Marshal(serverData{ID: int64(n), Label: "dob-test", Status: "running", Region: "us-east", IPv4: []string{"192.168.1.1", "8.8.8.8"}, Created: "2026-01-02T03:04:05", Tags: tags})
	return string(raw)
}
func key(t *testing.T) string {
	t.Helper()
	pub, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	p, e := ssh.NewPublicKey(pub)
	if e != nil {
		t.Fatal(e)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(p)))
}
func request(t *testing.T) providers.CreateServerRequest {
	return providers.CreateServerRequest{Name: "dob-test", RegionID: "us-east", PlanID: "g6-nanode-1", ImageID: "linode/ubuntu24.04", Identity: "dob-deployment-22222222-2222-4222-8222-222222222222", SSHAuthorizedKeys: []string{key(t)}, Tags: []string{"managed-by-digital-ocean-bot"}}
}

const accountJSON = `{"euuid":"external-account","email":"fixture@example.invalid","capabilities":["Linodes"],"balance":-1.25,"balance_uninvoiced":0.75}`

func TestFactoryAndFastObservation(t *testing.T) {
	if _, e := (Factory{}).Open(context.Background(), providers.OpenRequest{}); e == nil {
		t.Fatal("invalid open")
	}
	d := driver(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v4/account":
			return response(200, accountJSON), nil
		case "/v4/linode/instances":
			return response(200, page("["+server(1, nil)+"]", 1, 1, 1)), nil
		}
		t.Fatal("fast observation requested catalog")
		return nil, nil
	})
	cap := d.Capabilities()
	if !cap.InlineSSHKeys || cap.SSHKeys || !cap.Compute {
		t.Fatal(cap)
	}
	obs, e := d.ObserveFast(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if obs.Account.ID != "external-account" || obs.Capacity.LimitKnown || obs.Capacity.ComputeInUse != 1 || obs.Capacity.ComputeLimit != 0 || obs.Inventory.Servers[0].PrimaryIPv4 != "8.8.8.8" || !obs.Inventory.Servers[0].Ready || obs.Inventory.Servers[0].CreatedAt.IsZero() {
		t.Fatal(obs)
	}
	bill, e := d.Billing(context.Background())
	if e != nil || bill.Balance != "-1.25" || bill.MonthToDate != "0.75" || bill.InvoiceStatusKnown {
		t.Fatal(bill, e)
	}
}
func TestPaginationCompleteAndFaults(t *testing.T) {
	for _, tc := range []struct {
		name, first, second string
		ok                  bool
	}{
		{"empty", page("[]", 1, 1, 0), "", true},
		{"two-pages", page("["+server(1, nil)+"]", 1, 2, 2), page("["+server(2, nil)+"]", 2, 2, 2), true},
		{"duplicate", page("["+server(1, nil)+"]", 1, 2, 2), page("["+server(1, nil)+"]", 2, 2, 2), false},
		{"drift", page("["+server(1, nil)+"]", 1, 2, 2), page("["+server(2, nil)+"]", 2, 2, 3), false},
		{"missing", `{"page":1,"pages":1,"results":0}`, "", false},
		{"null", page("null", 1, 1, 0), "", false},
		{"truncated", page("[]", 1, 1, 1), "", false},
		{"wrong-page", page("[]", 2, 2, 0), "", false},
		{"bound", page("[]", 1, 201, 20001), "", false},
		{"bad-server", page(`[{"id":1}]`, 1, 1, 1), "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := driver(t, func(r *http.Request) (*http.Response, error) {
				body := tc.first
				if r.URL.Query().Get("page") == "2" {
					body = tc.second
				}
				return response(200, body), nil
			})
			inv, e := d.Inventory(context.Background())
			if (e == nil) != tc.ok {
				t.Fatal(inv, e)
			}
			if !tc.ok && inv.Servers != nil {
				t.Fatal("partial inventory leaked")
			}
		})
	}
}
func TestCreateInlineKeyAndIdentity(t *testing.T) {
	req := request(t)
	posts := 0
	var d *Driver
	d = driver(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v4/account/settings" {
			return response(200, `{"backups_enabled":false,"managed":false}`), nil
		}
		if r.Method == http.MethodGet {
			return response(200, page("["+server(7, []string{"managed-by-digital-ocean-bot", d.ownerTag(), identityTag(req.Identity)})+"]", 1, 1, 1)), nil
		}
		posts++
		var body map[string]any
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Fatal(e)
		}
		for _, field := range []string{"root_pass", "authorized_users", "metadata", "volumes", "enable_ipv6"} {
			if _, ok := body[field]; ok {
				t.Fatal("unexpected create field", field)
			}
		}
		if body["image"] != req.ImageID || body["booted"] != true || body["backups_enabled"] != false || body["private_ip"] != false {
			t.Fatal(body)
		}
		keys := body["authorized_keys"].([]any)
		if len(keys) != 1 || keys[0] != req.SSHAuthorizedKeys[0] {
			t.Fatal("SSH key mismatch")
		}
		tags := body["tags"].([]any)
		for _, tag := range tags {
			if len(tag.(string)) > 50 {
				t.Fatal("oversized tag")
			}
		}
		return response(200, server(7, nil)), nil
	})
	got, e := d.CreateServer(context.Background(), req)
	if e != nil || got.Outcome != providers.OutcomeAccepted || got.ServerID != "7" || posts != 1 {
		t.Fatal(got, e, posts)
	}
	matches, e := d.FindServerByIdentity(context.Background(), req.Identity)
	if e != nil || len(matches) != 1 || !contains(matches[0].Tags, req.Identity) {
		t.Fatal(matches, e)
	}
}
func TestCreateFaultsNeverRetryPOST(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		transport bool
		out       providers.MutationOutcome
	}{
		{"rate", 429, `{"errors":[{"reason":"fixture-token"}]}`, false, providers.OutcomeRejected},
		{"quota", 400, `{"errors":[{"reason":"Account Limit Reached. Please open a support ticket."}]}`, false, providers.OutcomeRejected},
		{"permission", 403, `{"errors":[{"reason":"fixture-token"}]}`, false, providers.OutcomeRejected},
		{"invalid-success", 200, "{}", false, providers.OutcomeAmbiguous},
		{"malformed-success", 200, "oops fixture-token", false, providers.OutcomeAmbiguous},
		{"provider-failure", 503, "fixture-token", false, providers.OutcomeAmbiguous},
		{"transport", 0, "", true, providers.OutcomeAmbiguous},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			d := driver(t, func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodGet {
					return response(200, `{"backups_enabled":false,"managed":false}`), nil
				}
				posts++
				if tc.transport {
					return nil, errors.New("fixture-token")
				}
				return response(tc.status, tc.body), nil
			})
			got, e := d.CreateServer(context.Background(), request(t))
			if e == nil || got.Outcome != tc.out || posts != 1 || strings.Contains(e.Error(), "fixture-token") {
				t.Fatal(got, e, posts)
			}
			if tc.name == "quota" && !providers.IsClass(e, providers.ErrorCapacity) {
				t.Fatal(e)
			}
		})
	}
}
func TestCreatePreflightDenials(t *testing.T) {
	for _, body := range []string{`{"backups_enabled":true,"managed":false}`, `{"backups_enabled":false,"managed":true}`, "{}"} {
		d := driver(t, func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Fatal("mutation before settings validation")
			}
			return response(200, body), nil
		})
		if _, e := d.CreateServer(context.Background(), request(t)); e == nil {
			t.Fatal("unsafe settings accepted")
		}
	}
	d := driver(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid input performed network call")
		return nil, nil
	})
	for _, mutate := range []func(*providers.CreateServerRequest){
		func(r *providers.CreateServerRequest) { r.Name = "a--b" },
		func(r *providers.CreateServerRequest) { r.SSHAuthorizedKeys = nil },
		func(r *providers.CreateServerRequest) {
			r.SSHAuthorizedKeys = []string{"command=\"bad\" " + r.SSHAuthorizedKeys[0]}
		},
		func(r *providers.CreateServerRequest) { r.SSHAuthorizedKeys[0] += "\n" + r.SSHAuthorizedKeys[0] },
		func(r *providers.CreateServerRequest) { r.ImageID = "private/1" },
	} {
		r := request(t)
		mutate(&r)
		if _, e := d.CreateServer(context.Background(), r); e == nil {
			t.Fatal("invalid request accepted")
		}
	}
}
func TestDeletionOwnershipAndAbsence(t *testing.T) {
	for _, kind := range []string{"owned", "foreign", "wrong-account", "absent", "permission", "locked"} {
		t.Run(kind, func(t *testing.T) {
			deleted := 0
			var d *Driver
			d = driver(t, func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodDelete {
					deleted++
					if kind == "locked" {
						return response(400, `{"errors":[{"reason":"cannot_delete"}]}`), nil
					}
					return response(200, "{}"), nil
				}
				if kind == "absent" {
					return response(404, "{}"), nil
				}
				if kind == "permission" {
					return response(403, "{}"), nil
				}
				tags := []string{"managed-by-digital-ocean-bot", d.ownerTag()}
				if kind == "foreign" {
					tags = nil
				}
				if kind == "wrong-account" {
					tags = []string{"managed-by-digital-ocean-bot", "dob-account-other"}
				}
				return response(200, server(7, tags)), nil
			})
			e := d.DeleteServer(context.Background(), "7")
			ok := kind == "owned" || kind == "absent"
			if (e == nil) != ok {
				t.Fatal(e)
			}
			if (kind == "owned" || kind == "locked") && deleted != 1 {
				t.Fatal(deleted)
			}
			if kind != "owned" && kind != "locked" && deleted != 0 {
				t.Fatal("foreign deletion")
			}
		})
	}
}
func TestTransportBoundsAndClassification(t *testing.T) {
	calls := 0
	d := driver(t, func(r *http.Request) (*http.Response, error) {
		calls++
		res := response(302, "")
		res.Header.Set("Location", "https://foreign.invalid")
		return res, nil
	})
	if _, e := d.Account(context.Background()); e == nil || calls != 1 {
		t.Fatal(e, calls)
	}
	for _, id := range []string{"../account", "0", "-1", "01", "1?x=y"} {
		if _, e := d.GetServer(context.Background(), id); e == nil {
			t.Fatal(id)
		}
	}
	if calls != 1 {
		t.Fatal("invalid IDs called API")
	}
	d = driver(t, func(*http.Request) (*http.Response, error) { return response(200, strings.Repeat("x", maxBody+1)), nil })
	if _, e := d.Account(context.Background()); !providers.IsClass(e, providers.ErrorUnavailable) {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		status int
		class  providers.ErrorClass
	}{{401, providers.ErrorAuthentication}, {403, providers.ErrorPermissionDenied}, {402, providers.ErrorBilling}, {429, providers.ErrorRateLimited}, {404, providers.ErrorNotFound}, {500, providers.ErrorUnavailable}} {
		pe := classify("op", tc.status, []byte(`{"errors":[{"reason":"fixture-token"}]}`))
		if pe.Class != tc.class || strings.Contains(pe.Error(), "fixture-token") {
			t.Fatal(pe)
		}
	}
	if got := retryAfter(http.Header{"Retry-After": []string{"9999999999"}}); got != time.Hour {
		t.Fatal(got)
	}
}
