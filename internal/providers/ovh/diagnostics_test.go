package ovh

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestOVHErrorCodesPreserveAuthenticationAndPermission(t *testing.T) {
	for _, tc := range []struct {
		status              int
		code, message, want string
		class               providers.ErrorClass
	}{
		{403, "INVALID_KEY", "private-secret", "INVALID_KEY", providers.ErrorAuthentication},
		{403, "INVALID_CREDENTIAL", "private-secret", "INVALID_CREDENTIAL", providers.ErrorAuthentication},
		{403, "NOT_CREDENTIAL", "private-secret", "NOT_CREDENTIAL", providers.ErrorAuthentication},
		{403, "NOT_GRANTED_CALL", "private-secret", "NOT_GRANTED_CALL", providers.ErrorPermissionDenied},
		{403, "FORBIDDEN", "private-secret", "FORBIDDEN", providers.ErrorPermissionDenied},
		{401, "INVALID_KEY", "private-secret", "AUTHENTICATION_FAILED", providers.ErrorAuthentication},
		{401, "NOT_GRANTED_CALL", "private-secret", "AUTHENTICATION_FAILED", providers.ErrorAuthentication},
		{403, "private-secret", "private-secret", "PERMISSION_DENIED", providers.ErrorPermissionDenied},
		{400, "", "Invalid signature", "INVALID_SIGNATURE", providers.ErrorAuthentication},
		{400, "", "Invalid timestamp", "INVALID_TIMESTAMP", providers.ErrorAuthentication},
		{500, "INVALID_KEY", "private-secret", "PROVIDER_UNAVAILABLE", providers.ErrorUnavailable},
		{429, "NOT_GRANTED_CALL", "private-secret", "RATE_LIMITED", providers.ErrorRateLimited},
	} {
		raw, _ := json.Marshal(map[string]string{"errorCode": tc.code, "message": tc.message})
		err := classify("/cloud/project", tc.status, raw)
		if err.Class != tc.class || Diagnostic(err).Code != tc.want || strings.Contains(err.Error(), "private-secret") {
			t.Fatalf("status=%d code=%s: class=%s diagnostic=%s", tc.status, tc.code, err.Class, Diagnostic(err).Code)
		}
	}
}

func TestOVHMalformedErrorCannotTriggerEndpointFallback(t *testing.T) {
	err := classify("/cloud/project", 403, []byte(`{"errorCode":"INVALID_KEY","message":`))
	if Diagnostic(err).Code != "PERMISSION_DENIED" || err.Class != providers.ErrorPermissionDenied {
		t.Fatal(err)
	}
}

func TestOVHDiscoveryDoesNotMaskPermissionWithAnotherEndpoint(t *testing.T) {
	for _, target := range []string{"eu.api.ovh.com", "ca.api.ovh.com", "api.us.ovhcloud.com"} {
		for _, code := range []string{"NOT_GRANTED_CALL", "FORBIDDEN", "INVALID_CREDENTIAL", "NOT_CREDENTIAL", "unrecognized"} {
			t.Run(target+"/"+code, func(t *testing.T) {
				hosts := []string{}
				h := transport(func(r *http.Request) (*http.Response, error) {
					if r.Method != "GET" {
						t.Fatal("discovery mutated provider")
					}
					if strings.HasSuffix(r.URL.Path, "/auth/time") {
						return response(readFixture(r.URL.Path)), nil
					}
					hosts = append(hosts, r.URL.Host)
					c := "INVALID_KEY"
					if r.URL.Host == target {
						c = code
					}
					x := response(`{"errorCode":"` + c + `","message":"never-print-this-secret"}`)
					x.StatusCode = 403
					return x, nil
				})
				_, err := (Factory{}).Discover(context.Background(), providers.OpenRequest{HTTPClient: &http.Client{Transport: h}, Credentials: testCredential(`{"application_key":"fixture-app","application_secret":"fixture-secret","consumer_key":"fixture-consumer"}`)}, nil)
				want := code
				if code == "unrecognized" {
					want = "PERMISSION_DENIED"
				}
				if err == nil || Diagnostic(err).Code != want || len(hosts) == 0 || hosts[len(hosts)-1] != target || strings.Contains(err.Error(), "never-print") {
					t.Fatal("lost definitive endpoint error", hosts, err)
				}
				d := Diagnostic(err)
				if d.Operation != "project_list" || d.RequiredRight != "GET /cloud/project" || endpoints[d.Endpoint] != "https://"+target+"/1.0" {
					t.Fatal(d)
				}
			})
		}
	}
	for _, endpoint := range []string{"", "ovh-eu"} {
		calls := 0
		h := transport(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "/auth/time") {
				return response(readFixture(r.URL.Path)), nil
			}
			calls++
			x := response(`{"errorCode":"INVALID_KEY"}`)
			x.StatusCode = 403
			return x, nil
		})
		token, _ := json.Marshal(credential{Endpoint: endpoint, ApplicationKey: "a", ApplicationSecret: "s", ConsumerKey: "c"})
		_, err := (Factory{}).Discover(context.Background(), providers.OpenRequest{HTTPClient: &http.Client{Transport: h}, Credentials: testCredential(token)}, nil)
		want := 3
		if endpoint != "" {
			want = 1
		}
		if calls != want || providers.Class(err) != providers.ErrorAuthentication || Diagnostic(err).Code != "INVALID_KEY" {
			t.Fatal(calls, err)
		}
	}
	project := strings.Repeat("a", 32)
	h := transport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/auth/time") {
			return response(readFixture(r.URL.Path)), nil
		}
		if r.URL.Path != "/1.0/cloud/project/"+project || r.Method != "GET" {
			t.Fatal("unexpected project-scoped request", r.URL.Path)
		}
		return response(`{"project_id":"` + project + `","status":"ok"}`), nil
	})
	d, err := (Factory{}).Discover(context.Background(), providers.OpenRequest{HTTPClient: &http.Client{Transport: h}, Credentials: testCredential(`{"application_key":"a","application_secret":"s","consumer_key":"c"}`)}, map[string]string{"endpoint": "ovh-eu", "project_id": project})
	if err != nil || !d.Complete {
		t.Fatal(d, err)
	}
}

func TestOVHDiagnosticRedactsRequestAndProviderData(t *testing.T) {
	project := strings.Repeat("b", 32)
	err := &requestError{endpoint: "ovh-us", method: "GET", path: "/cloud/project/" + project + "/flavor?region=private-secret", cause: &providers.Error{Class: providers.ErrorPermissionDenied, Code: "NOT_GRANTED_CALL", Operation: "private-secret", Message: "private-secret"}}
	d := Diagnostic(err)
	b, _ := json.Marshal(d)
	if strings.Contains(string(b), project) || strings.Contains(string(b), "private-secret") || d.RequiredRight != "GET /cloud/project/{project}/flavor" {
		t.Fatal(d)
	}
	for _, path := range []string{"/cloud/project/private-secret", "/cloud/project/" + project + "/private-secret", "/cloud/project/" + project + "/instance/private-secret"} {
		err.path = path
		d = Diagnostic(err)
		if d.Operation != "unknown" || d.RequiredRight != "" {
			t.Fatal(d)
		}
	}
	err.endpoint = "private-secret"
	err.method = "private-secret"
	err.cause = &providers.Error{Code: "private-secret", Class: providers.ErrorUnknown, Message: "private-secret", Cause: errors.New("private-secret")}
	d = Diagnostic(err)
	b, _ = json.Marshal(d)
	if strings.Contains(string(b), "private-secret") || d.Code != "REQUEST_FAILED" || d.Endpoint != "unknown" {
		t.Fatal(d)
	}
}
