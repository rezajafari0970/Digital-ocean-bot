package ovh

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestOVHNativeClassMessageErrorEnvelope(t *testing.T) {
	for _, tc := range []struct {
		message, code string
		class         providers.ErrorClass
	}{
		{"This application key is invalid", "INVALID_KEY", providers.ErrorAuthentication},
		{"This credential is not valid", "INVALID_CREDENTIAL", providers.ErrorAuthentication},
		{"This call has not been granted", "NOT_GRANTED_CALL", providers.ErrorPermissionDenied},
		{"User not granted for this request", "IAM_PERMISSION_DENIED", providers.ErrorPermissionDenied},
		{"Invalid signature", "INVALID_SIGNATURE", providers.ErrorAuthentication},
	} {
		raw, _ := json.Marshal(map[string]string{"class": "Client::Forbidden", "message": tc.message})
		e := classify("/cloud/project", 403, raw)
		if e.Code != tc.code || e.Class != tc.class {
			t.Fatal(tc.message, e)
		}
		kind, cls := errorResponseShape(raw)
		if kind != "class_message" || cls != "Client::Forbidden" {
			t.Fatal(kind, cls)
		}
	}
	// Native messages are interpreted only in the exact status/class envelope.
	for _, raw := range []string{
		`{"class":"unexpected","message":"This application key is invalid"}`,
		`{"message":"This application key is invalid"}`,
		`{"class":"Client::Forbidden","errorCode":"UNKNOWN","message":"This application key is invalid"}`,
		`{"class":"Client::Forbidden","message":"private-secret This application key is invalid"}`,
		`{"class":"Client::Forbidden","message":"This application key is invalid","bad":`,
	} {
		e := classify("/cloud/project", 403, []byte(raw))
		if e.Code != "PERMISSION_DENIED" || strings.Contains(e.Error(), "private-secret") {
			t.Fatal("ambiguous body accepted", e)
		}
	}
	for _, status := range []int{401, 429, 500} {
		e := classify("/cloud/project", status, []byte(`{"class":"Client::Forbidden","message":"This application key is invalid"}`))
		if e.Code == "INVALID_KEY" {
			t.Fatal("wrong HTTP status enabled fallback", status)
		}
	}
	e := classify("/cloud/project", 403, []byte(`{"class":"Client::Forbidden","errorCode":"NOT_GRANTED_CALL","message":"This application key is invalid"}`))
	if e.Code != "NOT_GRANTED_CALL" {
		t.Fatal("message overrode native errorCode", e)
	}
}

func TestOVHNativeEnvelopeDiscoveryAndScopedPermissions(t *testing.T) {
	for _, target := range []string{"eu.api.ovh.com", "ca.api.ovh.com", "api.us.ovhcloud.com"} {
		for _, message := range []string{"This call has not been granted", "This credential is not valid", "User not granted for this request"} {
			hosts := []string{}
			h := transport(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" {
					t.Fatal("mutation during discovery")
				}
				if strings.HasSuffix(r.URL.Path, "/auth/time") {
					return response(readFixture(r.URL.Path)), nil
				}
				hosts = append(hosts, r.URL.Host)
				msg := "This application key is invalid"
				if r.URL.Host == target {
					msg = message
				}
				raw, _ := json.Marshal(map[string]string{"class": "Client::Forbidden", "message": msg})
				x := response(string(raw))
				x.StatusCode = 403
				return x, nil
			})
			_, err := (Factory{}).Discover(context.Background(), providers.OpenRequest{HTTPClient: &http.Client{Transport: h}, Credentials: testCredential(`{"application_key":"fixture-app","application_secret":"fixture-secret","consumer_key":"fixture-consumer"}`)}, nil)
			d := Diagnostic(err)
			want := map[string]string{"This call has not been granted": "NOT_GRANTED_CALL", "This credential is not valid": "INVALID_CREDENTIAL", "User not granted for this request": "IAM_PERMISSION_DENIED"}[message]
			if err == nil || len(hosts) == 0 || hosts[len(hosts)-1] != target || d.Code != want || d.ResponseFormat != "class_message" || d.ResponseClass != "Client::Forbidden" || d.RequiredRight != "GET /cloud/project" {
				t.Fatal(target, d, hosts)
			}
		}
	}
	// Real native invalid-key responses permit reaching the correct endpoint.
	project := strings.Repeat("a", 32)
	h := transport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/auth/time") {
			return response(readFixture(r.URL.Path)), nil
		}
		if r.URL.Host != "api.us.ovhcloud.com" {
			x := response(`{"class":"Client::Forbidden","message":"This application key is invalid"}`)
			x.StatusCode = 403
			return x, nil
		}
		if strings.HasSuffix(r.URL.Path, "/cloud/project") {
			return response(`["` + project + `"]`), nil
		}
		return response(`{"project_id":"` + project + `","status":"ok"}`), nil
	})
	d, err := (Factory{}).Discover(context.Background(), providers.OpenRequest{HTTPClient: &http.Client{Transport: h}, Credentials: testCredential(`{"application_key":"fixture-app","application_secret":"fixture-secret","consumer_key":"fixture-consumer"}`)}, nil)
	if err != nil || !d.Complete || d.Resolved["endpoint"] != "ovh-us" {
		t.Fatal(d, err)
	}
}

func TestOVHResponseShapeNeverExposesUnknownData(t *testing.T) {
	for _, tc := range []struct{ raw, format, cls string }{
		{"<html>private-secret</html>", "non_json", "unknown"},
		{`{"class":"private-secret","message":"private-secret"}`, "class_message", "unknown"},
		{`{"errorCode":"private-secret","message":"private-secret"}`, "error_code", "unknown"},
		{`{"class":42,"message":"private-secret"}`, "json_other", "unknown"},
		{`["private-secret"]`, "json_other", "unknown"},
		{`null`, "json_other", "unknown"},
		{`{"class":"Client::Forbidden","message":"private-secret"}`, "class_message", "Client::Forbidden"},
	} {
		format, cls := errorResponseShape([]byte(tc.raw))
		if format != tc.format || cls != tc.cls || strings.Contains(format+cls, "private-secret") {
			t.Fatal(format, cls)
		}
	}
	d := Diagnostic(&requestError{cause: problem(providers.ErrorPermissionDenied, "project", "PERMISSION_DENIED", 403), responseFormat: "private-secret", responseClass: "private-secret"})
	if strings.Contains(d.ResponseFormat+d.ResponseClass, "private-secret") {
		t.Fatal("metadata leaked")
	}
}
