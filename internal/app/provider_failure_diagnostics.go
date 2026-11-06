package app

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/aws"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ovh"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/upcloud"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/redact"
)

// Only bounded, credential-free diagnostic context is persisted. This evidence
// neither changes admission nor replays a provider operation.
type ProviderFailureEvidence struct {
	Provider          string `json:"provider"`
	Class             string `json:"error_class,omitempty"`
	HTTPStatus        int    `json:"http_status,omitempty"`
	Code              string `json:"provider_code,omitempty"`
	Operation         string `json:"operation,omitempty"`
	Guidance          string `json:"guidance,omitempty"`
	RetryAfterSeconds int64  `json:"retry_after_seconds,omitempty"`
}

var failureCode = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,96}$`)
var failureURL = regexp.MustCompile(`(?i)\b(?:https?|socks5?h?)://[^\s]+`)

func safeFailureText(s string) string {
	s = redact.Text(failureURL.ReplaceAllString(s, "[connection address redacted]"))
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 800 {
		s = string(r[:800]) + "…"
	}
	return s
}
func safeFailureOperation(op string) string {
	known := strings.Fields("credential discovery account catalog inventory network server get_server create_server delete_server find_server list_servers list_images list_regions list_sizes list_ssh_keys create_ssh_key delete_ssh_key clock billing account_read snapshot identity")
	for _, v := range known {
		if op == v {
			return v
		}
	}
	if !strings.HasPrefix(op, "/") {
		return "provider_request"
	}
	op = strings.SplitN(op, "?", 2)[0]
	parts := strings.Split(op, "/")
	if len(parts) > 10 {
		return "provider_request"
	}
	allowed := "/account/profile/linode/instances/types/regions/images/v2/v4/v1/ssh-keys/sshkeys/servers/server/zones/plans/cloud/public/project/me/balance/"
	for i, p := range parts {
		if p != "" && !strings.Contains(allowed, "/"+p+"/") {
			parts[i] = ":resource"
		}
	}
	return strings.Join(parts, "/")
}
func providerFailureReceipt(provider string, cause error, state, detail string) string {
	var existing struct {
		Message string `json:"provider_error"`
	}
	if strings.HasPrefix(strings.TrimSpace(detail), "{") {
		if json.Unmarshal([]byte(detail), &existing) == nil {
			detail = existing.Message
		} else {
			detail = ""
		}
	}
	if detail == "" {
		detail = cause.Error()
	}
	d := ProviderFailureEvidence{Provider: provider}
	var pe *providers.Error
	if errors.As(cause, &pe) {
		d.Class = string(pe.Class)
		if pe.StatusCode >= 100 && pe.StatusCode <= 599 {
			d.HTTPStatus = pe.StatusCode
		}
		if failureCode.MatchString(pe.Code) {
			d.Code = pe.Code
		}
		d.Operation = safeFailureOperation(pe.Operation)
		if pe.RetryAfter > 0 {
			d.RetryAfterSeconds = int64(min(pe.RetryAfter, 24*time.Hour) / time.Second)
		}
	}
	switch state {
	case "TOKEN_INVALID":
		d.Guidance = "Authentication was rejected. Check the credential set, expiry and provider API access; the response alone does not establish which is wrong."
	case "PERMISSION_DENIED":
		d.Guidance = "The provider denied this operation. Check credential permissions and account/project policy."
	case "LOCKED":
		d.Guidance = "The provider reports an account restriction. Review its console or support notice."
	case "BILLING_BLOCKED":
		d.Guidance = "Resolve the provider billing restriction before retrying."
	case "RATE_LIMITED":
		d.Guidance = "Wait for the provider retry interval before retrying."
	case "TRANSPORT_ERROR", "PROXY_ERROR":
		d.Guidance = "Verify the assigned route, proxy country and provider reachability. No direct-route fallback is authorized."
	default:
		d.Guidance = "Review the recorded operation and provider response code before retrying."
	}
	if provider == "aws" {
		v := aws.Diagnostic(cause)
		d.Code = v.Code
		if v.Operation != "unknown" {
			d.Operation = v.Operation
		}
		if v.Guidance != "" {
			d.Guidance = v.Guidance
		}
	} else if provider == "ovh" {
		v := ovh.Diagnostic(cause)
		d.Code = v.Code
		if v.Operation != "unknown" {
			d.Operation = v.Operation
		}
		if v.Guidance != "" {
			d.Guidance = v.Guidance
		}
	} else if provider == "upcloud" && d.Code == "" {
		d.Code = upcloud.Diagnostic(cause)
	} else if provider == "vultr" && state == "TOKEN_INVALID" {
		d.Guidance = "Vultr rejected authentication. Check the API key, API access and source-IP allowlist against the selected proxy exit; HTTP 401 alone does not distinguish them."
	}
	var readiness networkReadinessError
	if errors.As(cause, &readiness) {
		d.Code = string(readiness)
		d.Class = "network_readiness"
		d.Operation = "network"
		d.Guidance = readiness.guidance() + ". Requests remain blocked until verification succeeds; no direct fallback is authorized."
	}
	raw, _ := json.Marshal(struct {
		State      string                  `json:"provider_state"`
		Message    string                  `json:"provider_error"`
		CanCreate  bool                    `json:"can_create"`
		Diagnostic ProviderFailureEvidence `json:"diagnostic"`
	}{state, safeFailureText(detail), false, d})
	return string(raw)
}
