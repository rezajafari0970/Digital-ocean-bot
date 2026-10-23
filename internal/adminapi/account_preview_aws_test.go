package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/aws"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAWSPreviewActionableDiagnosticsAndSecretBoundary(t *testing.T) {
	for _, tc := range []struct {
		code   string
		status int
	}{{"InvalidParameterValue", 422}, {"IncompleteSignature", 401}, {"UnauthorizedOperation", 403}, {"Blocked", 403}, {"OptInRequired", 403}, {"PendingVerification", 403}, {"MissingAuthenticationToken", 401}, {"AuthFailure", 401}} {
		t.Run(tc.code, func(t *testing.T) {
			calls := 0
			h := quotaDiagnosticTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.URL.Query().Get("Action") != "DescribeRegions" {
					t.Fatal("unexpected or mutating request")
				}
				return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`<Response><Errors><Error><Code>` + tc.code + `</Code><Message>private-secret</Message></Error></Errors></Response>`)), Request: r}, nil
			})
			_, err := (aws.Factory{}).Discover(context.Background(), providers.OpenRequest{AccountID: "preview", HTTPClient: &http.Client{Transport: h}, Credentials: previewCredential{[]byte(`{"access_key_id":"private-secret","secret_access_key":"private-secret"}`)}}, nil)
			if err == nil || calls != 1 {
				t.Fatal("failed discovery accepted or replayed")
			}
			var logs bytes.Buffer
			old := log.Writer()
			log.SetOutput(&logs)
			defer log.SetOutput(old)
			w := httptest.NewRecorder()
			writeStagedProviderPreviewError(w, err, previewDiagnosticContext{Provider: "aws", Stage: "discovery", RequestID: "aws-trace", Started: time.Now()})
			var body map[string]any
			if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != tc.status || body["diagnostic"] != tc.code || body["operation"] != "DescribeRegions" || body["required_permission"] != "ec2:DescribeRegions" || body["response_format"] != "ec2_xml" {
				t.Fatal(w.Code, w.Body.String())
			}
			for _, out := range []string{logs.String(), w.Body.String()} {
				if strings.Contains(out, "private-secret") {
					t.Fatal("secret leaked")
				}
				if !strings.Contains(out, "aws-trace") || !strings.Contains(out, tc.code) {
					t.Fatal("trace lost")
				}
			}
		})
	}
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	w := httptest.NewRecorder()
	writeStagedProviderPreviewError(w, &providers.Error{Class: providers.ErrorPermissionDenied, Code: "private-secret", Operation: "private-secret", Message: "private-secret", Cause: errors.New("private-secret")}, previewDiagnosticContext{Provider: "aws", Stage: "bad\nprivate-secret", RequestID: "bad\nprivate-secret", Started: time.Now()})
	if strings.Contains(logs.String()+w.Body.String(), "private-secret") {
		t.Fatal("injected metadata leaked")
	}
}
