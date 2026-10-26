package adminapi

import (
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/aws"
	"log"
	"net/http"
	"time"
)

func writeAWSPreviewError(w http.ResponseWriter, err error, c previewDiagnosticContext) {
	d := aws.Diagnostic(err)
	class := providers.Class(err)
	status, code := 422, "provider_validation_failed"
	switch class {
	case providers.ErrorAuthentication:
		status, code = 401, "provider_credential_invalid"
	case providers.ErrorPermissionDenied, providers.ErrorAccountLocked:
		status, code = 403, "provider_permission_denied"
	case providers.ErrorRateLimited:
		status, code = 429, "provider_rate_limited"
	case providers.ErrorTransport:
		status, code = 503, "provider_transport_failed"
	case providers.ErrorUnavailable:
		status, code = 502, "provider_response_unavailable"
	}
	providerStatus := 0
	var pe *providers.Error
	if errors.As(err, &pe) && pe.StatusCode >= 100 && pe.StatusCode <= 599 {
		providerStatus = pe.StatusCode
	}
	id, stage := previewCode(c.RequestID), previewCode(c.Stage)
	log.Printf("provider_preview_failed provider=aws request_id=%s stage=%s class=%s operation=%s provider_status=%d diagnostic=%s response_format=%s elapsed_ms=%d", id, stage, previewCode(string(class)), d.Operation, providerStatus, d.Code, d.ResponseFormat, time.Since(c.Started).Milliseconds())
	detail := fmt.Sprintf("AWS %s. %s Stage: %s; operation: %s", d.Code, d.Guidance, stage, d.Operation)
	if d.RequiredPermission != "" {
		detail += "; required permission: " + d.RequiredPermission
	}
	if providerStatus != 0 {
		detail += fmt.Sprintf("; HTTP %d", providerStatus)
	}
	detail += "; ref: " + id + "."
	writeJSON(w, status, map[string]any{"error": code, "detail": detail, "request_id": id, "stage": stage, "operation": d.Operation, "provider_status": providerStatus, "diagnostic": d.Code, "response_format": d.ResponseFormat, "identity_verified": d.IdentityVerified, "required_permission": d.RequiredPermission})
}
