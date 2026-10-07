package jobs

import (
	"strconv"
	"strings"
)

// normalizeDiagnostic accepts only the bounded grammar emitted by the executor.
// Legacy recovery rows may contain raw provider responses. Preserve that stored
// evidence when no replacement was supplied, but never return it on Operation.
func normalizeDiagnostic(code, message string) (string, string) {
	if code == "" && message == "" {
		return "", ""
	}
	switch code {
	case "unknown", "billing", "authentication", "permission_denied", "account_locked",
		"rate_limited", "capacity", "region_capacity", "image_unavailable", "not_found",
		"invalid_request", "transport", "unavailable", "ambiguous_outcome", "mutation_blocked",
		"deadline_exceeded", "cancelled", "recovery_retry", "delete_without_provider_resource",
		"stale_create_without_provider_resource":
	default:
		code = "unknown"
	}
	for _, stage := range []string{"operation", "pre_create", "create_server", "create_rejected", "create_ambiguous", "create_egress", "delete_pre_egress", "delete_server", "delete_egress", "recovery"} {
		prefix := stage + ": " + code
		if message == prefix {
			return code, message
		}
		rest, ok := strings.CutPrefix(message, prefix+" (HTTP ")
		if !ok || len(rest) != 4 || rest[3] != ')' {
			continue
		}
		status, err := strconv.Atoi(rest[:3])
		if err == nil && status >= 100 && status <= 599 {
			return code, message
		}
	}
	return code, "operation: " + code
}

func (o *Operation) normalizeDiagnostics() {
	if o.State == OperationSucceeded {
		o.ErrorCode, o.ErrorMessage = "", ""
		return
	}
	o.ErrorCode, o.ErrorMessage = normalizeDiagnostic(o.ErrorCode, o.ErrorMessage)
}
