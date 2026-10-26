package adminapi

import (
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestKamateraPreviewErrorsKeepStageAndNeverEchoSecrets(t *testing.T) {
	for _, tc := range []struct {
		class  providers.ErrorClass
		status int
		code   string
	}{
		{providers.ErrorTransport, 503, "provider_transport_failed"}, {providers.ErrorUnavailable, 502, "provider_response_unavailable"}, {providers.ErrorAuthentication, 401, "provider_credential_invalid"}, {providers.ErrorPermissionDenied, 403, "provider_permission_denied"},
	} {
		w := httptest.NewRecorder()
		writeStagedProviderPreviewError(w, &providers.Error{Class: tc.class, StatusCode: 503, Operation: "capabilities", Code: "PROVIDER_UNAVAILABLE", Message: "private-secret", Cause: errors.New("private-secret")}, previewDiagnosticContext{Provider: "kamatera", Stage: "catalog", RequestID: "trace-123", Started: time.Now()})
		s := w.Body.String()
		if w.Code != tc.status || !strings.Contains(s, tc.code) || !strings.Contains(s, "capabilities") || !strings.Contains(s, "trace-123") || strings.Contains(s, "private-secret") {
			t.Fatal(w.Code, s)
		}
	}
	w := httptest.NewRecorder()
	writeStagedProviderPreviewError(w, &providers.Error{Class: providers.ErrorUnknown, Operation: "private-secret", Code: "private-secret", Message: "private-secret"}, previewDiagnosticContext{Provider: "kamatera", Stage: "catalog", RequestID: "trace", Started: time.Now()})
	if strings.Contains(w.Body.String(), "private-secret") {
		t.Fatal(w.Body.String())
	}
}
