package adminapi

import (
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"log"
	"net/http"
	"time"
)

func writeKamateraPreviewError(w http.ResponseWriter, err error, c previewDiagnosticContext) {
	status, code, detail := 422, "provider_validation_failed", "Kamatera could not complete this validation step."
	switch providers.Class(err) {
	case providers.ErrorAuthentication:
		status, code, detail = 401, "provider_credential_invalid", "Kamatera rejected the API client ID and secret."
	case providers.ErrorPermissionDenied, providers.ErrorAccountLocked:
		status, code, detail = 403, "provider_permission_denied", "Kamatera denied access to this operation."
	case providers.ErrorRateLimited:
		status, code, detail = 429, "provider_rate_limited", "Kamatera rate limit reached. Retry later."
	case providers.ErrorTransport:
		status, code, detail = 503, "provider_transport_failed", "The selected connection failed while contacting Kamatera."
	case providers.ErrorUnavailable:
		status, code, detail = 502, "provider_response_unavailable", "Kamatera replied, but this step could not be completed."
	}
	op, reason, upstream := "unknown", "REQUEST_FAILED", 0
	var pe *providers.Error
	if errors.As(err, &pe) {
		switch pe.Operation {
		case "credential", "account", "catalog", "request", "response", "inventory", "regions", "plans", "images", "capabilities":
			op = pe.Operation
		}
		switch pe.Code {
		case "INVALID_CREDENTIAL", "CREDENTIAL_SCOPE_CHANGED", "CREDENTIAL_REJECTED", "ACCESS_DENIED", "RATE_LIMITED", "TRANSPORT_ERROR", "PROVIDER_UNAVAILABLE", "INVALID_RESPONSE", "INVALID_REGIONS", "INVALID_PLANS", "INVALID_IMAGES", "INVALID_IMAGE_SCOPE", "INVALID_CAPABILITIES", "NO_SUPPORTED_UBUNTU_PLANS":
			reason = pe.Code
		}
		if pe.StatusCode >= 100 && pe.StatusCode <= 599 {
			upstream = pe.StatusCode
		}
	}
	id, stage := previewCode(c.RequestID), previewCode(c.Stage)
	detail += fmt.Sprintf(" Stage: %s; operation: %s; reason: %s", stage, op, reason)
	if upstream > 0 {
		detail += fmt.Sprintf("; HTTP %d", upstream)
	}
	detail += "; ref: " + id + "."
	log.Printf("provider_preview_failed provider=kamatera request_id=%s stage=%s operation=%s diagnostic=%s provider_status=%d elapsed_ms=%d", id, stage, op, reason, upstream, time.Since(c.Started).Milliseconds())
	writeJSON(w, status, map[string]any{"error": code, "detail": detail, "request_id": id, "stage": stage, "operation": op, "diagnostic": reason, "provider_status": upstream})
}
