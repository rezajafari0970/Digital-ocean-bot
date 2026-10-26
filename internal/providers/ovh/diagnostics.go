package ovh

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

// requestError carries local request context without changing the provider
// error contract or exposing credentials, identifiers, query strings or bodies.
type requestError struct {
	cause                         error
	endpoint, method, path        string
	responseFormat, responseClass string
}

func (e *requestError) Error() string { return e.cause.Error() }
func (e *requestError) Unwrap() error { return e.cause }

type DiagnosticInfo struct {
	Code, Endpoint, Operation, RequiredRight, Guidance string
	ResponseFormat, ResponseClass                      string
}

func Diagnostic(err error) DiagnosticInfo {
	d := DiagnosticInfo{Code: "REQUEST_FAILED", Endpoint: "unknown", Operation: "unknown", ResponseFormat: "none", ResponseClass: "unknown"}
	var pe *providers.Error
	if errors.As(err, &pe) {
		switch pe.Code {
		case "INVALID_KEY", "INVALID_CREDENTIAL", "NOT_CREDENTIAL", "NOT_GRANTED_CALL", "FORBIDDEN",
			"AUTHORIZATION_OUTCOME_UNKNOWN", "AUTHORIZATION_TRANSPORT_UNKNOWN", "AUTHORIZATION_BODY_UNKNOWN", "AUTHORIZATION_SERVER_UNKNOWN", "AUTHORIZATION_REDIRECT_UNKNOWN", "AUTHORIZATION_JSON_UNKNOWN", "AUTHORIZATION_GRANT_UNKNOWN", "AUTHORIZATION_URL_UNKNOWN", "AUTHORIZATION_REFUSED_OR_EXPIRED", "AUTHORIZATION_RIGHTS_INCOMPLETE", "IAM_PERMISSION_DENIED", "INVALID_SIGNATURE", "INVALID_TIMESTAMP", "AUTHENTICATION_FAILED", "PERMISSION_DENIED",
			"INVALID_CREDENTIAL_JSON", "INVALID_PROJECT_ID", "PROJECT_SELECTION_REQUIRED",
			"NO_ACTIVE_PUBLIC_CLOUD_PROJECT", "PROJECT_NOT_ACTIVE_OR_ACCESSIBLE",
			"PROJECT_ID_MISMATCH", "PROJECT_SCOPE_CHANGED", "PROJECT_SCOPE_MISMATCH",
			"CLOCK_UNAVAILABLE", "INVALID_CLOCK", "CLOCK_SKEW_EXCESSIVE",
			"TRANSPORT_ERROR", "INVALID_RESPONSE_BODY", "INVALID_RESPONSE", "MISSING_COLLECTION",
			"INVALID_COLLECTION_IDENTITY", "RATE_LIMITED", "BILLING_BLOCKED", "NOT_FOUND",
			"PROVIDER_UNAVAILABLE", "REQUEST_REJECTED", "COMPUTE_QUOTA_EXCEEDED", "FLAVOR_QUOTA_EXCEEDED", "REGIONAL_QUOTA_EXCEEDED", "REGION_CAPACITY_UNAVAILABLE":
			d.Code = pe.Code
		}
	}
	var re *requestError
	if errors.As(err, &re) {
		switch re.responseFormat {
		case "none", "non_json", "json_other", "class_message", "error_code":
			d.ResponseFormat = re.responseFormat
		}
		d.ResponseClass = safeResponseClass(re.responseClass)
		if endpoints[re.endpoint] != "" {
			d.Endpoint = re.endpoint
		}
		d.Operation, d.RequiredRight = safeRequest(re.method, re.path)
		if pe != nil && pe.Operation == "clock" {
			d.Operation, d.RequiredRight = "clock", ""
		}
	}
	switch d.Code {
	case "AUTHORIZATION_OUTCOME_UNKNOWN":
		d.Guidance = "The credential request outcome is unknown. It will not be sent again automatically. Review OVH API keys before explicitly starting a new request."
	case "AUTHORIZATION_TRANSPORT_UNKNOWN":
		d.Guidance = "The connection ended before an OVH response was available. Review the selected connection and OVH API keys before starting a new request."
	case "AUTHORIZATION_BODY_UNKNOWN":
		d.Guidance = "The OVH response was incomplete or exceeded the response limit. Review OVH API keys before starting a new request."
	case "AUTHORIZATION_SERVER_UNKNOWN":
		d.Guidance = "OVH returned a server error after submission. Review OVH API keys before starting a new request."
	case "AUTHORIZATION_REDIRECT_UNKNOWN":
		d.Guidance = "The credential endpoint returned a redirect. The request was not forwarded. Review the endpoint and OVH API keys."
	case "AUTHORIZATION_JSON_UNKNOWN":
		d.Guidance = "The credential endpoint did not return valid JSON. Review the selected connection and OVH API keys."
	case "AUTHORIZATION_GRANT_UNKNOWN":
		d.Guidance = "The OVH response did not contain a valid pending credential. Review OVH API keys before starting a new request."
	case "AUTHORIZATION_URL_UNKNOWN":
		d.Guidance = "The approval URL did not match a supported official OVH authorization route. Review OVH API keys and report this request reference."
	case "AUTHORIZATION_REFUSED_OR_EXPIRED":
		d.Guidance = "The authorization was refused or expired. Start a new connection and approve its permissions in OVH."
	case "AUTHORIZATION_RIGHTS_INCOMPLETE":
		d.Guidance = "OVH approved fewer rights than this connection requires. Start a new connection and approve the displayed project and instance permissions."
	case "INVALID_KEY":
		d.Guidance = "Application key is not recognized at this endpoint. Check the OVH region where the keys were created and copy all three keys from the same set."
	case "INVALID_CREDENTIAL", "NOT_CREDENTIAL":
		d.Guidance = "Consumer key is invalid, expired, revoked or not yet validated. Validate or replace the Consumer key for this application in OVH."
	case "NOT_GRANTED_CALL":
		d.Guidance = "The Consumer key does not grant this API call. Create or authorize a key with the required method and path."
		if d.Operation == "project_list" {
			d.Guidance += " Automatic discovery needs GET /cloud/project. For a deliberately project-scoped key, enter Endpoint and Project ID in Advanced overrides."
		}
	case "IAM_PERMISSION_DENIED":
		d.Guidance = "OVH account/user IAM policy does not grant this request. Grant access to the selected Public Cloud project in the OVH IAM policy; Consumer key rules alone may not be enough."
	case "FORBIDDEN":
		d.Guidance = "OVH forbids this request. Check the key access rules, account/user permissions and any source-IP restriction for the selected connection."
	case "PERMISSION_DENIED":
		d.Guidance = "HTTP 403 did not include a recognized OVH API reason. Check the selected connection and endpoint; this alone does not prove that the key is invalid."
	case "INVALID_SIGNATURE":
		d.Guidance = "OVH rejected the request signature. Verify that Application key, Application secret and Consumer key belong to the same key set."
	case "INVALID_TIMESTAMP":
		d.Guidance = "OVH rejected the request timestamp. Check clock synchronization and connection latency."
	case "NO_ACTIVE_PUBLIC_CLOUD_PROJECT":
		d.Guidance = "No active Public Cloud project was returned. Create or activate a Public Cloud project in OVH and grant this key access."
	case "INVALID_CREDENTIAL_JSON":
		d.Guidance = "Enter all three OVH keys. If an Endpoint override is set, use ovh-eu, ovh-ca or ovh-us."
	case "NOT_FOUND":
		d.Guidance = "The requested resource was not found. Check the Endpoint and Project ID overrides."
	}
	return d
}

// Return only literal route templates. Never echo provider-supplied paths,
// project IDs, instance IDs, query parameters or arbitrary metadata.
func safeRequest(method, path string) (string, string) {
	if method != "GET" && method != "POST" && method != "DELETE" {
		return "unknown", ""
	}
	path = strings.SplitN(path, "?", 2)[0]
	if method == "POST" && path == "/auth/credential" {
		return "authorization_request", ""
	}
	if method == "GET" && path == "/auth/currentCredential" {
		return "authorization_check", "GET /auth/currentCredential"
	}
	if method == "GET" && path == "/cloud/project" {
		return "project_list", "GET /cloud/project"
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 3 || parts[0] != "cloud" || parts[1] != "project" || !projectPattern.MatchString(parts[2]) {
		return "unknown", ""
	}
	base := method + " /cloud/project/{project}"
	if len(parts) == 3 && method == "GET" {
		return "project_read", base
	}
	if len(parts) == 4 {
		switch parts[3] {
		case "region", "flavor", "image", "quota":
			if method == "GET" {
				return parts[3] + "_list", base + "/" + parts[3]
			}
		case "instance":
			if method == "GET" {
				return "instance_list", base + "/instance"
			}
			if method == "POST" {
				return "instance_create", base + "/instance"
			}
		}
	}
	if len(parts) == 5 {
		if parts[3] == "region" && method == "GET" && regionPattern.MatchString(parts[4]) {
			return "region_read", base + "/region/*"
		}
		if parts[3] == "instance" && uuidPattern.MatchString(parts[4]) {
			if method == "GET" {
				return "instance_read", base + "/instance/*"
			}
			if method == "DELETE" {
				return "instance_delete", base + "/instance/*"
			}
		}
	}
	return "unknown", ""
}

// Preserve response shape for diagnosis without retaining raw messages or
// headers. This distinguishes a native API error from a non-JSON denial.
func errorResponseShape(raw []byte) (string, string) {
	if !json.Valid(raw) {
		return "non_json", "unknown"
	}
	var x struct {
		Class string `json:"class"`
		Code  string `json:"errorCode"`
	}
	if json.Unmarshal(raw, &x) != nil {
		return "json_other", "unknown"
	}
	format := "json_other"
	if x.Class != "" {
		format = "class_message"
	} else if x.Code != "" {
		format = "error_code"
	}
	return format, safeResponseClass(x.Class)
}
func safeResponseClass(class string) string {
	switch class {
	case "Client::Forbidden", "Client::Unauthorized", "Client::BadRequest",
		"Client::NotFound", "Client::Conflict", "Client::TooManyRequests",
		"Server::InternalServerError", "Server::ServiceUnavailable":
		return class
	}
	return "unknown"
}
