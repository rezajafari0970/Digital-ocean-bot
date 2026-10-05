package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/upcloud"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUpCloudPreviewStageAndSecretRedaction(t *testing.T) {
	secret := "ucat_do-not-print-or-persist"
	for _, tc := range []struct {
		class  providers.ErrorClass
		status int
		want   string
	}{
		{providers.ErrorTransport, 503, "selected connection failed"},
		{providers.ErrorUnavailable, 502, "UpCloud replied"},
		{providers.ErrorAuthentication, 401, "rejected the API token"},
		{providers.ErrorPermissionDenied, 403, "does not permit"},
		{providers.ErrorRateLimited, 429, "rate limit"},
	} {
		t.Run(string(tc.class), func(t *testing.T) {
			var logs bytes.Buffer
			old := log.Writer()
			log.SetOutput(&logs)
			defer log.SetOutput(old)
			err := &providers.Error{Class: tc.class, Operation: "catalog_plans", StatusCode: 200, Message: secret, Cause: errors.New("socks5://user:" + secret + "@proxy.invalid")}
			w := httptest.NewRecorder()
			writeStagedProviderPreviewError(w, err, previewDiagnosticContext{Provider: "upcloud", Stage: "catalog", RequestID: "trace-123", Started: time.Now()})
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.want) || !strings.Contains(w.Body.String(), "catalog_plans") {
				t.Fatal(w.Code, w.Body.String())
			}
			for _, out := range []string{w.Body.String(), logs.String()} {
				if strings.Contains(out, secret) || strings.Contains(out, "proxy.invalid") {
					t.Fatal("secret leaked")
				}
			}
			if !strings.Contains(logs.String(), "request_id=trace-123 stage=catalog") {
				t.Fatal(logs.String())
			}
		})
	}
}
func TestUpCloudPreviewRejectsInjectedMetadata(t *testing.T) {
	w := httptest.NewRecorder()
	writeStagedProviderPreviewError(w, &providers.Error{Class: providers.ErrorTransport, Operation: "Bearer secret\nforged"}, previewDiagnosticContext{Provider: "upcloud", Stage: "catalog\nforged", RequestID: "bad\ntrace", Started: time.Now()})
	if strings.Contains(w.Body.String(), "forged") || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Body.String())
	}
}

type quotaDiagnosticTransport func(*http.Request) (*http.Response, error)

func (f quotaDiagnosticTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUpCloudPreviewReportsAllSafeQuotaIssues(t *testing.T) {
	client := &http.Client{Transport: quotaDiagnosticTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"account":{"username":"private-user","resource_limits":{"cores":0.5,"memory":null,"ucat_secret-key":"ucat_secret-value"}}}`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	d, err := (upcloud.Factory{}).Open(context.Background(), providers.OpenRequest{AccountID: "fixture", HTTPClient: client, Credentials: previewCredential{[]byte("ucat_secret-token")}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.(providers.AccountReader).Account(context.Background())
	if err == nil {
		t.Fatal("invalid quota accepted")
	}
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	w := httptest.NewRecorder()
	writeStagedProviderPreviewError(w, err, previewDiagnosticContext{Provider: "upcloud", Stage: "account", RequestID: "quota-trace", Started: time.Now()})
	var body struct {
		Diagnostic string   `json:"diagnostic"`
		Issues     []string `json:"quota_issues"`
		Status     int      `json:"provider_status"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
		t.Fatal(e)
	}
	if w.Code != 502 || body.Status != 200 || body.Diagnostic != "INVALID_QUOTA_CORES_DECIMAL_NUMBER" || len(body.Issues) != 3 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, issue := range body.Issues {
		if !strings.Contains(logs.String(), issue) {
			t.Fatal("journal lost quota issue")
		}
	}
	for _, out := range []string{logs.String(), w.Body.String()} {
		for _, secret := range []string{"ucat_secret", "private-user", "0.5"} {
			if strings.Contains(out, secret) {
				t.Fatal("secret/value exposed")
			}
		}
	}
}
