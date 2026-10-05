package adminapi

import (
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/upcloud"
	"log"
	"net/http"
	"regexp"
	"time"
)

type previewDiagnosticContext struct {
	Provider, Stage, RequestID, ExitIP string
	Started                            time.Time
}

var previewSafeCode = regexp.MustCompile("^[A-Za-z0-9_.-]{1,96}$")

func previewCode(s string) string {
	if previewSafeCode.MatchString(s) {
		return s
	}
	return "unknown"
}

func writeStagedProviderPreviewError(w http.ResponseWriter, err error, c previewDiagnosticContext) {
	if c.Provider != "upcloud" {
		writeProviderPreviewError(w, err, c.ExitIP)
		return
	}
	class := providers.Class(err)
	status := 0
	operation := "unknown"
	var pe *providers.Error
	if errors.As(err, &pe) {
		operation = previewCode(pe.Operation)
		if pe.StatusCode >= 100 && pe.StatusCode <= 599 {
			status = pe.StatusCode
		}
	}
	code := previewCode(upcloud.Diagnostic(err))
	stage := previewCode(c.Stage)
	id := previewCode(c.RequestID)
	detail := "UpCloud could not complete this validation step."
	httpStatus, errorCode := 422, "provider_validation_failed"
	switch class {
	case providers.ErrorAuthentication:
		httpStatus, errorCode = 401, "provider_credential_invalid"
		detail = "UpCloud rejected the API token."
	case providers.ErrorPermissionDenied, providers.ErrorAccountLocked:
		httpStatus, errorCode = 403, "provider_permission_denied"
		detail = "The UpCloud token or account does not permit this operation."
	case providers.ErrorBilling:
		httpStatus, errorCode = 402, "provider_billing_blocked"
		detail = "UpCloud requires account credit for this operation."
	case providers.ErrorRateLimited:
		httpStatus, errorCode = 429, "provider_rate_limited"
		detail = "UpCloud rate limit reached. Retry later."
	case providers.ErrorTransport:
		httpStatus, errorCode = 503, "provider_transport_failed"
		detail = "The selected connection failed while contacting UpCloud."
	case providers.ErrorUnavailable:
		httpStatus, errorCode = 502, "provider_response_unavailable"
		detail = "UpCloud replied, but this step could not be completed."
	}
	// All metadata is generated locally or bounded; never log err.Error(), raw
	// provider responses, token, proxy credentials or request/response headers.
	elapsed := time.Since(c.Started).Milliseconds()
	quotaIssues := upcloud.QuotaIssues(err)
	log.Printf("provider_preview_failed provider=upcloud request_id=%s stage=%s class=%s operation=%s provider_status=%d diagnostic=%s elapsed_ms=%d quota_issues=%v", id, stage, previewCode(string(class)), operation, status, code, elapsed, quotaIssues)
	detail += fmt.Sprintf(" Stage: %s; operation: %s; reason: %s", stage, operation, code)
	if status != 0 {
		detail += fmt.Sprintf("; HTTP %d", status)
	}
	detail += "; ref: " + id + "."
	writeJSON(w, httpStatus, map[string]any{"error": errorCode, "detail": detail, "request_id": id, "stage": stage, "operation": operation, "provider_status": status, "diagnostic": code, "quota_issues": quotaIssues})
}
