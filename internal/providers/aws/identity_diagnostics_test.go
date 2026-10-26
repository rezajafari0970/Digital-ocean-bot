package aws

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAWSIdentityPrecedesRegionalAccessAndSeparatesFailure(t *testing.T) {
	for _, identityOK := range []bool{false, true} {
		calls := []string{}
		request := providers.OpenRequest{AccountID: "preview", Credentials: testCredential(`{"access_key_id":"fixture-access","secret_access_key":"fixture-secret"}`), HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			action := query(r).Get("Action")
			calls = append(calls, action)
			if action == "GetCallerIdentity" && identityOK {
				return response(action, networkFixture(action)), nil
			}
			code := "InvalidClientTokenId"
			raw := "<ErrorResponse><Error><Code>" + code + "</Code></Error></ErrorResponse>"
			if action == "DescribeRegions" {
				raw = "<Response><Errors><Error><Code>AuthFailure</Code><Message>secret-must-not-leak</Message></Error></Errors></Response>"
			}
			return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(raw)), Header: make(http.Header)}, nil
		})}}
		_, err := (Factory{}).Discover(context.Background(), request, nil)
		if err == nil {
			t.Fatal("unexpected success")
		}
		d := Diagnostic(err)
		if len(calls) == 0 || calls[0] != "GetCallerIdentity" || d.IdentityVerified != identityOK || d.RequiredPermission != "" {
			t.Fatalf("identity evidence: %+v calls=%v", d, calls)
		}
		if identityOK && (len(calls) != 2 || providers.Class(err) != providers.ErrorPermissionDenied || d.Code != "AuthFailure") {
			t.Fatal("EC2 error misclassified", d, calls)
		}
		if !identityOK && (len(calls) != 1 || providers.Class(err) != providers.ErrorAuthentication) {
			t.Fatal("continued after rejected identity", calls)
		}
		if strings.Contains(err.Error()+d.Guidance, "secret-must-not-leak") {
			t.Fatal("raw response leaked")
		}
	}
}
func TestAWSSharedCredentialPasteAndTemporaryValidation(t *testing.T) {
	c, e := signingCredentials(context.Background(), testCredential(`{"access_key_id":"  ASIAfixture  ","secret_access_key":" fixture-secret ","session_token":" token ","region":" eu-west-1 "}`))
	if e != nil || c.AccessKey != "ASIAfixture" || c.SecretKey != "fixture-secret" || c.SessionToken != "token" {
		t.Fatal("paste normalization failed", e)
	}
	for _, raw := range []string{`{"access_key_id":"ASIAfixture","secret_access_key":"secret","region":"eu-west-1"}`, `{"access_key_id":"key","secret_access_key":"sec ret","region":"eu-west-1"}`, `{"access_key_id":"key","secret_access_key":"secret","session_token":"to\nken","region":"eu-west-1"}`} {
		if _, err := signingCredentials(context.Background(), testCredential(raw)); err == nil {
			t.Fatal("invalid signing credential accepted")
		}
	}
}

func TestAWSRuntimeIdentityEvidenceDoesNotInvalidateVerifiedKeys(t *testing.T) {
	calls := []string{}
	c := &client{http: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		action := query(r).Get("Action")
		calls = append(calls, action)
		if action == "GetCallerIdentity" {
			return response(action, fixtureRead(action)), nil
		}
		return &http.Response{StatusCode: 403, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`<Response><Errors><Error><Code>AuthFailure</Code><Message>private-secret</Message></Error></Errors></Response>`))}, nil
	})}, credentials: testCredential(fixtureCredential), region: "eu-west-1", expected: "123456789012:eu-west-1"}
	err := c.call(context.Background(), "ec2", "DescribeInstances", nil, false, nil)
	d := Diagnostic(err)
	if len(calls) != 2 || calls[0] != "GetCallerIdentity" || !d.IdentityVerified || d.Operation != "DescribeInstances" || d.Code != "AuthFailure" || d.RequiredPermission != "" || providers.Class(err) != providers.ErrorPermissionDenied {
		t.Fatal("runtime identity diagnosis", d, err, calls)
	}
	if strings.Contains(err.Error(), "private-secret") {
		t.Fatal("raw response leaked")
	}
}
