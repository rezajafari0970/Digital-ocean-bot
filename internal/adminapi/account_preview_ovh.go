package adminapi

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ovh"
)

func writeOVHPreviewError(w http.ResponseWriter, err error, c previewDiagnosticContext) {
	d := ovh.Diagnostic(err)
	class := providers.Class(err)
	httpStatus, code := 422, "provider_validation_failed"
	switch class {
	case providers.ErrorAuthentication:
		httpStatus, code = 401, "provider_credential_invalid"
	case providers.ErrorPermissionDenied, providers.ErrorAccountLocked:
		httpStatus, code = 403, "provider_permission_denied"
	case providers.ErrorBilling:
		httpStatus, code = 402, "provider_billing_blocked"
	case providers.ErrorRateLimited:
		httpStatus, code = 429, "provider_rate_limited"
	case providers.ErrorTransport:
		httpStatus, code = 503, "provider_transport_failed"
	case providers.ErrorUnavailable:
		httpStatus, code = 502, "provider_response_unavailable"
	}
	status := 0
	var pe *providers.Error
	if errors.As(err, &pe) && pe.StatusCode >= 100 && pe.StatusCode <= 599 {
		status = pe.StatusCode
	}
	id, stage := previewCode(c.RequestID), previewCode(c.Stage)
	// Diagnostic returns only local allowlisted values; raw errors, keys,
	// headers, provider messages and proxy addresses are never logged.
	log.Printf("provider_preview_failed provider=ovh request_id=%s stage=%s class=%s endpoint=%s operation=%s provider_status=%d diagnostic=%s response_format=%s response_class=%s elapsed_ms=%d", id, stage, previewCode(string(class)), d.Endpoint, d.Operation, status, d.Code, d.ResponseFormat, d.ResponseClass, time.Since(c.Started).Milliseconds())
	detail := fmt.Sprintf("OVH %s. %s Stage: %s; endpoint: %s; operation: %s", d.Code, d.Guidance, stage, d.Endpoint, d.Operation)
	if d.RequiredRight != "" {
		detail += "; required right: " + d.RequiredRight
	}
	if status != 0 {
		detail += fmt.Sprintf("; HTTP %d", status)
	}
	detail += "; ref: " + id + "."
	writeJSON(w, httpStatus, map[string]any{"error": code, "detail": detail, "request_id": id, "stage": stage, "endpoint": d.Endpoint, "operation": d.Operation, "provider_status": status, "diagnostic": d.Code, "required_right": d.RequiredRight, "response_format": d.ResponseFormat, "response_class": d.ResponseClass})
}
