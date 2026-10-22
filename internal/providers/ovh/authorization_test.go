package ovh

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOVHAuthorizationDelegationAndActualRights(t *testing.T) {
	ctx := context.Background()
	calls := 0
	project := strings.Repeat("a", 32)
	for _, scope := range []string{"", project} {
		req := providers.OpenRequest{Credentials: testCredential(`{"application_key":"fixture-app","application_secret":"fixture-secret","project_id":"` + scope + `"}`)}
		req.HTTPClient = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != "POST" || r.URL.Path != "/1.0/auth/credential" || r.Header.Get("X-Ovh-Application") != "fixture-app" || r.Header.Get("X-Ovh-Signature") != "" || r.Header.Get("X-Ovh-Consumer") != "" {
				t.Fatal("invalid delegation request")
			}
			raw, _ := io.ReadAll(r.Body)
			if strings.Contains(string(raw), "fixture-secret") {
				t.Fatal("secret sent to unsigned endpoint")
			}
			var body struct {
				Rules []accessRule `json:"accessRules"`
			}
			if json.Unmarshal(raw, &body) != nil {
				t.Fatal("invalid body")
			}
			want, _ := json.Marshal(authorizationRules(scope))
			got, _ := json.Marshal(body.Rules)
			if string(want) != string(got) {
				t.Fatal("wrong permission scope")
			}
			return response(`{"state":"pendingValidation","consumerKey":"fixture-consumer","validationUrl":"https://eu.api.ovh.com/auth/?credentialToken=fixture-validation"}`), nil
		})}
		grant, e := (Factory{}).StartAuthorization(ctx, req)
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(grant)
		if strings.Contains(string(raw), "fixture-") {
			t.Fatal("grant serialized a secret")
		}
		req.Credentials = grant.Credentials
		for _, status := range []string{"pendingValidation", "validated", "refused", "expired", "unexpected"} {
			for _, complete := range []bool{false, true} {
				req.HTTPClient = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
					if r.Method != "GET" {
						t.Fatal("check mutated provider")
					}
					if strings.HasSuffix(r.URL.Path, "/auth/time") {
						return response(readFixture(r.URL.Path)), nil
					}
					if r.URL.Path != "/1.0/auth/currentCredential" || r.Header.Get("X-Ovh-Signature") == "" {
						t.Fatal("invalid activation check")
					}
					rules := authorizationRules(scope)
					if !complete {
						rules = rules[:len(rules)-1]
					}
					b, _ := json.Marshal(map[string]any{"status": status, "rules": rules})
					return response(string(b)), nil
				})}
				ready, e := (Factory{}).CheckAuthorization(ctx, req)
				if status == "pendingValidation" {
					if ready || e != nil {
						t.Fatal("pending rejected", e)
					}
				} else if status == "validated" && complete {
					if !ready || e != nil {
						t.Fatal("approved grant rejected", e)
					}
				} else if ready || e == nil {
					t.Fatal("incomplete/refused grant accepted", status, complete)
				}
			}
		}
	}
	if calls != 2 {
		t.Fatal("unexpected credential dispatches", calls)
	}
}
func TestOVHAuthorizationUnknownOutcomeAndURLBoundary(t *testing.T) {
	for _, url := range []string{"http://eu.api.ovh.com/auth/?credentialToken=fixture-token", "https://eu.api.ovh.com.evil.invalid/auth/?credentialToken=fixture-token", "https://eu.api.ovh.com@evil.invalid/auth/?credentialToken=fixture-token", "https://eu.api.ovh.com:444/auth/?credentialToken=fixture-token", "https://eu.api.ovh.com/redirect?credentialToken=fixture-token", "https://eu.api.ovh.com/auth/?credentialToken=fixture-token&redirect=https://evil.invalid", "https://eu.api.ovh.com/auth/?credentialToken=fixture-token#evil", "https://api.us.ovhcloud.com/auth/?credentialToken=fixture-token"} {
		if validAuthorizationURL(url, "ovh-eu") {
			t.Fatal("untrusted approval URL", url)
		}
	}
	for _, mode := range []string{"transport", "500", "redirect", "invalid-json", "foreign-url"} {
		calls := 0
		req := providers.OpenRequest{Credentials: testCredential(`{"application_key":"fixture-app","application_secret":"fixture-secret"}`), HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			calls++
			switch mode {
			case "transport":
				return nil, errors.New("secret diagnostic")
			case "500":
				x := response("{}")
				x.StatusCode = 500
				return x, nil
			case "redirect":
				x := response("")
				x.StatusCode = 302
				x.Header = http.Header{"Location": []string{"https://evil.invalid/"}}
				return x, nil
			case "foreign-url":
				return response(`{"state":"pendingValidation","consumerKey":"fixture-consumer","validationUrl":"https://evil.invalid/auth/?credentialToken=fixture-token"}`), nil
			}
			return response("{"), nil
		})}}
		_, e := (Factory{}).StartAuthorization(context.Background(), req)
		if providers.Class(e) != providers.ErrorAmbiguousOutcome || calls != 1 || strings.Contains(e.Error(), "secret diagnostic") {
			t.Fatal("ambiguous dispatch retried or leaked", mode, e, calls)
		}
	}
}
func TestOVHAuthorizationEndpointFallbackAndExpiry(t *testing.T) {
	calls := 0
	req := providers.OpenRequest{Credentials: testCredential(`{"application_key":"fixture-app","application_secret":"fixture-secret"}`), HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host == "eu.api.ovh.com" {
			x := response(`{"errorCode":"INVALID_KEY"}`)
			x.StatusCode = 403
			return x, nil
		}
		return response(`{"state":"pendingValidation","consumerKey":"fixture-consumer","validationUrl":"https://ca.api.ovh.com/auth/?credentialToken=fixture-token"}`), nil
	})}}
	grant, e := (Factory{}).StartAuthorization(context.Background(), req)
	if e != nil || calls != 2 {
		t.Fatal("endpoint discovery failed", e, calls)
	}
	req.Credentials = grant.Credentials
	req.HTTPClient = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/auth/time") {
			return response(readFixture(r.URL.Path)), nil
		}
		b, _ := json.Marshal(map[string]any{"status": "validated", "rules": authorizationRules(""), "expiration": time.Now().Add(-time.Minute)})
		return response(string(b)), nil
	})}
	if ready, e := (Factory{}).CheckAuthorization(context.Background(), req); ready || e == nil {
		t.Fatal("expired approval accepted")
	}
	req.Credentials = testCredential(`{"application_key":"fixture-app"}`)
	calls = 0
	if _, e := (Factory{}).StartAuthorization(context.Background(), req); e == nil || calls != 0 {
		t.Fatal("application key alone accepted")
	}
}

func TestOVHAuthorizationOfficialRoutesAndDiagnostics(t *testing.T) {
	var fixtures []struct {
		Endpoint string `json:"endpoint"`
		URL      string `json:"url"`
		Valid    bool   `json:"valid"`
	}
	raw, e := os.ReadFile("testdata/authorization-urls.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &fixtures); e != nil {
		t.Fatal(e)
	}
	for _, f := range fixtures {
		if got := validAuthorizationURL(f.URL, f.Endpoint); got != f.Valid {
			t.Fatalf("URL acceptance mismatch: %q %s got %v", f.URL, f.Endpoint, got)
		}
		if !f.Valid {
			continue
		}
		calls := 0
		c, _ := json.Marshal(map[string]string{"endpoint": f.Endpoint, "application_key": "fixture-app", "application_secret": "fixture-secret"})
		req := providers.OpenRequest{Credentials: testCredential(c), HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			calls++
			payload, _ := json.Marshal(map[string]string{"state": "pendingValidation", "consumerKey": "fixture-consumer", "validationUrl": f.URL})
			return response(string(payload)), nil
		})}}
		grant, e := (Factory{}).StartAuthorization(context.Background(), req)
		if e != nil || calls != 1 || grant.ValidationURL != f.URL {
			t.Fatalf("official approval route lost: %s %v calls=%d", f.Endpoint, e, calls)
		}
	}
	for _, f := range []struct {
		body, code string
		status     int
	}{
		{"{", "AUTHORIZATION_JSON_UNKNOWN", 200},
		{`{"state":"validated","consumerKey":"fixture-consumer"}`, "AUTHORIZATION_GRANT_UNKNOWN", 200},
		{`{"state":"pendingValidation","consumerKey":"fixture-consumer","validationUrl":"https://evil.invalid/?credentialToken=private-token"}`, "AUTHORIZATION_URL_UNKNOWN", 200},
		{strings.Repeat("s", 65537), "AUTHORIZATION_BODY_UNKNOWN", 200},
		{"secret-server-body", "AUTHORIZATION_SERVER_UNKNOWN", 503},
		{"secret-redirect-body", "AUTHORIZATION_REDIRECT_UNKNOWN", 307},
	} {
		calls := 0
		req := providers.OpenRequest{Credentials: testCredential(`{"application_key":"fixture-app","application_secret":"fixture-secret"}`), HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			calls++
			resp := response(f.body)
			resp.StatusCode = f.status
			return resp, nil
		})}}
		_, e := (Factory{}).StartAuthorization(context.Background(), req)
		d := Diagnostic(e)
		var pe *providers.Error
		if calls != 1 || providers.Class(e) != providers.ErrorAmbiguousOutcome || !errors.As(e, &pe) || pe.StatusCode != f.status || d.Code != f.code || d.Endpoint != "ovh-eu" || d.Operation != "authorization_request" || d.Guidance == "" {
			t.Fatalf("diagnostic/replay boundary failed: %+v %v", d, e)
		}
		if strings.Contains(e.Error()+d.Guidance, "private-token") || strings.Contains(e.Error()+d.Guidance, "secret-server-body") {
			t.Fatal("response leaked")
		}
	}
}
