package vultr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestMutationContracts(t *testing.T) {
	var createSeen, keySeen bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/instances":
			var x createInstanceRequest
			if json.NewDecoder(r.Body).Decode(&x) != nil {
				t.Fatal("bad body")
			}
			if x.Region != "ewr" || x.Plan != "vc2" || x.OSID != 2284 || x.EnableIPv6 || x.UserData == "" || len(x.SSHKeyIDs) != 1 || x.SSHKeyIDs[0] != "k1" || !containsString(x.Tags, "identity-1") {
				t.Fatalf("create=%+v", x)
			}
			createSeen = true
			w.WriteHeader(202)
			_, _ = w.Write([]byte(`{"instance":{"id":"v1","status":"pending","region":"ewr","plan":"vc2","label":"n","tags":["identity-1"]}}`))
		case r.Method == "GET" && r.URL.Path == "/instances":
			_, _ = w.Write([]byte(`{"instances":[{"id":"v1","status":"active","main_ip":"203.0.113.9","region":"ewr","plan":"vc2","label":"n","tags":["identity-1"]}]}`))
		case r.Method == "DELETE" && r.URL.Path == "/instances/v1":
			w.WriteHeader(204)
		case r.Method == "POST" && r.URL.Path == "/ssh-keys":
			var x map[string]string
			_ = json.NewDecoder(r.Body).Decode(&x)
			if x["name"] != "k" || x["ssh_key"] != "ssh-ed25519 AAA" {
				t.Fatalf("key=%v", x)
			}
			keySeen = true
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"ssh_key":{"id":"k1","name":"k","ssh_key":"ssh-ed25519 AAA"}}`))
		case r.Method == "GET" && r.URL.Path == "/ssh-keys/k1":
			_, _ = w.Write([]byte(`{"ssh_key":{"id":"k1","name":"k","ssh_key":"ssh-ed25519 AAAATEST unit@test"}}`))
		case r.Method == "DELETE" && r.URL.Path == "/ssh-keys/k1":
			w.WriteHeader(204)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer s.Close()
	c := NewClient(s.Client(), "token")
	c.base = s.URL
	d := &Driver{client: c}
	ctx := context.Background()
	got, err := d.CreateServer(ctx, providers.CreateServerRequest{Name: "n", RegionID: "ewr", PlanID: "vc2", ImageID: "2284", SSHKeyRefs: []string{"k1"}, Identity: "identity-1"})
	if err != nil || got.ServerID != "v1" || got.Outcome != providers.OutcomeAccepted || !createSeen {
		t.Fatalf("create=%+v err=%v", got, err)
	}
	found, err := d.FindServerByIdentity(ctx, "identity-1")
	if err != nil || len(found) != 1 || found[0].ID != "v1" {
		t.Fatalf("found=%+v err=%v", found, err)
	}
	if err = d.DeleteServer(ctx, "v1"); err != nil {
		t.Fatal(err)
	}
	k, err := d.CreateSSHKey(ctx, "k", "ssh-ed25519 AAA")
	if err != nil || k.ID != "k1" || !keySeen {
		t.Fatalf("key=%+v err=%v", k, err)
	}
	if err = d.DeleteSSHKey(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
}
func TestCreate429IsAmbiguousWithRetryAfter(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"rate limit"}`))
	}))
	defer s.Close()
	c := NewClient(s.Client(), "t")
	c.base = s.URL
	d := &Driver{client: c}
	got, err := d.CreateServer(context.Background(), providers.CreateServerRequest{RegionID: "ewr", PlanID: "vc2", ImageID: "2284"})
	if err == nil || got.Outcome != providers.OutcomeAmbiguous {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	var pe *providers.Error
	if !providers.IsClass(err, providers.ErrorRateLimited) || !errorsAs(err, &pe) || pe.RetryAfter != 7*time.Second {
		t.Fatalf("err=%+v", err)
	}
}
func TestDelete404IsIdempotent(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer s.Close()
	c := NewClient(s.Client(), "t")
	c.base = s.URL
	d := &Driver{client: c}
	if err := d.DeleteServer(context.Background(), "gone"); err != nil {
		t.Fatal(err)
	}
}
func errorsAs(err error, target any) bool { return errors.As(err, target) }

func TestCreate502IsAmbiguousAndNotRetried(t *testing.T) {
	var calls int
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("{\"error\":\"upstream maintenance\"}"))
	}))
	defer s.Close()
	c := NewClient(s.Client(), "t")
	c.base = s.URL
	d := &Driver{client: c}
	got, err := d.CreateServer(context.Background(), providers.CreateServerRequest{RegionID: "ewr", PlanID: "vc2", ImageID: "2284"})
	if err == nil || got.Outcome != providers.OutcomeAmbiguous || !providers.IsClass(err, providers.ErrorTransport) {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if calls != 1 {
		t.Fatalf("mutation retried %d times", calls)
	}
}

func TestCreateClientTimeoutIsAmbiguousAndNotRetried(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(80 * time.Millisecond)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer s.Close()
	hc := s.Client()
	hc.Timeout = 20 * time.Millisecond
	c := NewClient(hc, "t")
	c.base = s.URL
	d := &Driver{client: c}
	got, err := d.CreateServer(context.Background(), providers.CreateServerRequest{RegionID: "ewr", PlanID: "vc2", ImageID: "2284"})
	if err == nil || got.Outcome != providers.OutcomeAmbiguous || !providers.IsClass(err, providers.ErrorTransport) {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if gotCalls := calls.Load(); gotCalls != 1 {
		t.Fatalf("timed-out mutation retried %d times", gotCalls)
	}
}

func TestDelete502ReturnsAmbiguousProviderError(t *testing.T) {
	var calls int
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer s.Close()
	c := NewClient(s.Client(), "t")
	c.base = s.URL
	d := &Driver{client: c}
	err := d.DeleteServer(context.Background(), "v1")
	if err == nil || !providers.IsClass(err, providers.ErrorTransport) {
		t.Fatalf("err=%v", err)
	}
	if calls != 1 {
		t.Fatalf("delete mutation retried %d times", calls)
	}
}
