package adminapi

import (
	"bytes"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"log"
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
