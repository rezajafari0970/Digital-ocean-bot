package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ovh"
)

func TestOVHPreviewActionableDiagnosticsAndSecretBoundary(t *testing.T) {
	for _, tc := range []struct {
		code string
		want int
	}{{"NOT_GRANTED_CALL", 403}, {"INVALID_CREDENTIAL", 401}, {"INVALID_KEY", 401}, {"FORBIDDEN", 403}} {
		t.Run(tc.code, func(t *testing.T) {
			h := quotaDiagnosticTransport(func(r *http.Request) (*http.Response, error) {
				body := `{"errorCode":"` + tc.code + `","message":"private-secret"}`
				status := 403
				if strings.HasSuffix(r.URL.Path, "/auth/time") {
					body = strconv.FormatInt(time.Now().Unix(), 10)
					status = 200
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			_, err := (ovh.Factory{}).Discover(context.Background(), providers.OpenRequest{HTTPClient: &http.Client{Transport: h}, Credentials: previewCredential{[]byte(`{"endpoint":"ovh-us","application_key":"private-secret","application_secret":"private-secret","consumer_key":"private-secret"}`)}}, nil)
			if err == nil {
				t.Fatal("rejected response accepted")
			}
			var logs bytes.Buffer
			old := log.Writer()
			log.SetOutput(&logs)
			defer log.SetOutput(old)
			w := httptest.NewRecorder()
			writeStagedProviderPreviewError(w, err, previewDiagnosticContext{Provider: "ovh", Stage: "discovery", RequestID: "ovh-trace", Started: time.Now()})
			var body map[string]any
			if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != tc.want || body["diagnostic"] != tc.code || body["endpoint"] != "ovh-us" || body["operation"] != "project_list" || body["required_right"] != "GET /cloud/project" {
				t.Fatal(w.Code, w.Body.String())
			}
			for _, out := range []string{w.Body.String(), logs.String()} {
				if strings.Contains(out, "private-secret") {
					t.Fatal("secret leaked")
				}
				if !strings.Contains(out, "ovh-trace") || !strings.Contains(out, tc.code) {
					t.Fatal("missing trace")
				}
			}
			if tc.code == "NOT_GRANTED_CALL" && !strings.Contains(w.Body.String(), "Advanced overrides") {
				t.Fatal("missing recovery guidance")
			}
		})
	}
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	w := httptest.NewRecorder()
	writeStagedProviderPreviewError(w, &providers.Error{Class: providers.ErrorPermissionDenied, Code: "private-secret", Operation: "private-secret", Message: "private-secret", Cause: errors.New("private-secret")}, previewDiagnosticContext{Provider: "ovh", Stage: "bad\nprivate-secret", RequestID: "bad\nprivate-secret", Started: time.Now()})
	if strings.Contains(w.Body.String()+logs.String(), "private-secret") {
		t.Fatal("injected metadata leaked")
	}
}

func TestOVHPreviewNativeEnvelopeDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		message, code string
		status        int
	}{
		{"This application key is invalid", "INVALID_KEY", 401},
		{"This credential is not valid", "INVALID_CREDENTIAL", 401},
		{"This call has not been granted", "NOT_GRANTED_CALL", 403},
		{"User not granted for this request", "IAM_PERMISSION_DENIED", 403},
		{"Invalid signature", "INVALID_SIGNATURE", 401},
		{"private-secret", "PERMISSION_DENIED", 403},
	} {
		t.Run(tc.code, func(t *testing.T) {
			h := quotaDiagnosticTransport(func(r *http.Request) (*http.Response, error) {
				raw, _ := json.Marshal(map[string]string{"class": "Client::Forbidden", "message": tc.message, "private": "private-secret"})
				body, status := string(raw), 403
				if strings.HasSuffix(r.URL.Path, "/auth/time") {
					body = strconv.FormatInt(time.Now().Unix(), 10)
					status = 200
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			_, err := (ovh.Factory{}).Discover(context.Background(), providers.OpenRequest{HTTPClient: &http.Client{Transport: h}, Credentials: previewCredential{[]byte(`{"endpoint":"ovh-eu","application_key":"private-secret","application_secret":"private-secret","consumer_key":"private-secret"}`)}}, nil)
			if err == nil {
				t.Fatal("rejected response accepted")
			}
			var logs bytes.Buffer
			old := log.Writer()
			log.SetOutput(&logs)
			defer log.SetOutput(old)
			w := httptest.NewRecorder()
			writeStagedProviderPreviewError(w, err, previewDiagnosticContext{Provider: "ovh", Stage: "discovery", RequestID: "native-trace", Started: time.Now()})
			var body map[string]any
			if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != tc.status || body["diagnostic"] != tc.code || body["response_format"] != "class_message" || body["response_class"] != "Client::Forbidden" || body["endpoint"] != "ovh-eu" || body["required_right"] != "GET /cloud/project" {
				t.Fatal(w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String()+logs.String(), "private-secret") {
				t.Fatal("secret leaked")
			}
			for _, value := range []string{"response_format=class_message", "response_class=Client::Forbidden", "native-trace", "diagnostic=" + tc.code} {
				if !strings.Contains(logs.String(), value) {
					t.Fatal("missing diagnostic", value)
				}
			}
			if tc.code == "IAM_PERMISSION_DENIED" && !strings.Contains(w.Body.String(), "IAM policy") {
				t.Fatal("missing IAM guidance")
			}
		})
	}
}
