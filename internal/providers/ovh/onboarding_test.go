package ovh

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"strings"
	"testing"
)

func TestOVHAutomaticDiscoveryAndProjectChoice(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		calls := 0
		project := strings.Repeat("a", 32)
		other := strings.Repeat("b", 32)
		h := transport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != "GET" {
				t.Fatal("preview mutation")
			}
			if strings.HasSuffix(r.URL.Path, "/auth/time") {
				return response(readFixture(r.URL.Path)), nil
			}
			if r.URL.Host != "api.us.ovhcloud.com" {
				x := response(`{"errorCode":"INVALID_KEY"}`)
				x.StatusCode = 401
				return x, nil
			}
			if r.Header.Get("X-Ovh-Signature") == "" {
				t.Fatal("unsigned discovery")
			}
			if strings.HasSuffix(r.URL.Path, "/cloud/project") {
				body := `["` + project + `"]`
				if multiple {
					body = `["` + other + `","` + project + `"]`
				}
				return response(body), nil
			}
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			return response(`{"project_id":"` + id + `","status":"ok","description":"Public Cloud"}`), nil
		})
		req := providers.OpenRequest{AccountID: "preview", HTTPClient: &http.Client{Transport: h}, Credentials: testCredential(`{"application_key":"fixture-app","application_secret":"fixture-secret","consumer_key":"fixture-consumer"}`)}
		d, e := (Factory{}).Discover(context.Background(), req, nil)
		if e != nil || d.Complete == multiple || d.Resolved["endpoint"] != "ovh-us" {
			t.Fatal(d, e)
		}
		b, _ := json.Marshal(d)
		if strings.Contains(string(b), "fixture-") {
			t.Fatal("secret returned")
		}
		if multiple {
			if len(d.Fields[0].Options) != 2 {
				t.Fatal(d)
			}
			d, e = (Factory{}).Discover(context.Background(), req, map[string]string{"endpoint": "ovh-us", "project_id": project})
			if e != nil || !d.Complete {
				t.Fatal(d, e)
			}
		}
		c, e := credentials(context.Background(), d.Credentials)
		if e != nil || c.Project != project || c.Endpoint != "ovh-us" || c.ApplicationSecret != "fixture-secret" {
			t.Fatal("normalized credentials invalid", e)
		}
		if calls < 6 {
			t.Fatal(calls)
		}
	}
}
func TestOVHDiscoveryFailsClosed(t *testing.T) {
	for _, body := range []string{`null`, `["invalid"]`, `[]`, `["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]`} {
		req := providers.OpenRequest{HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "/auth/time") {
				return response(readFixture(r.URL.Path)), nil
			}
			return response(body), nil
		})}, Credentials: testCredential(`{"endpoint":"ovh-eu","application_key":"a","application_secret":"s","consumer_key":"c"}`)}
		if _, e := (Factory{}).Discover(context.Background(), req, nil); e == nil {
			t.Fatal("invalid list accepted", body)
		}
	}
	calls := 0
	req := providers.OpenRequest{HTTPClient: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("secret transport detail")
	})}, Credentials: testCredential(`{"application_key":"a","application_secret":"s","consumer_key":"c"}`)}
	if _, e := (Factory{}).Discover(context.Background(), req, nil); e == nil || calls != 1 || strings.Contains(e.Error(), "secret transport") {
		t.Fatal("transport treated as wrong endpoint", e, calls)
	}
	req.Credentials = testCredential(`{"application_key":"a"}`)
	calls = 0
	if _, e := (Factory{}).Discover(context.Background(), req, nil); e == nil || calls != 0 {
		t.Fatal("one key accepted", e, calls)
	}
}
