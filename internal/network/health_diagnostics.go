package network

import (
	"context"
	"errors"
	"net"
)

// Health diagnostics are a closed vocabulary. Raw transport errors can contain
// credentials, request URLs and query tokens and must never become stored text.
func NormalizeHealthDiagnostic(code string) string {
	switch code {
	case "", "PROXY_AUTH_FAILED", "PROXY_AUTH_METHOD_UNSUPPORTED",
		"PROXY_TIMEOUT", "PROXY_CHECK_CANCELED", "PROXY_CONFIG_INVALID",
		"PROXY_ENDPOINT_HTTP_FAILED", "PROXY_ENDPOINT_PAYLOAD_INVALID",
		"PROXY_EXIT_IP_MISMATCH", "PROXY_SERVER_IP_LEAK", "PROXY_HEADER_LEAK",
		"PROXY_TRANSPORT_FAILED", "PROXY_HEALTH_FAILED", "PROXY_LATENCY_EXCEEDED",
		"PROXY_SECRET_UNAVAILABLE":
		return code
	case "secret unavailable":
		return "PROXY_SECRET_UNAVAILABLE"
	default:
		return "PROXY_HEALTH_FAILED"
	}
}

func activeProbeDiagnostic(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrProxyAuth):
		return "PROXY_AUTH_FAILED"
	case errors.Is(err, context.Canceled):
		return "PROXY_CHECK_CANCELED"
	case errors.Is(err, context.DeadlineExceeded):
		return "PROXY_TIMEOUT"
	case errors.Is(err, ErrProxyConfigInvalid), errors.Is(err, ErrUnsupportedProxyType):
		return "PROXY_CONFIG_INVALID"
	case errors.Is(err, ErrHealthHTTP):
		return "PROXY_ENDPOINT_HTTP_FAILED"
	case errors.Is(err, ErrHealthPayload):
		return "PROXY_ENDPOINT_PAYLOAD_INVALID"
	case errors.Is(err, ErrUnexpectedExitIP):
		return "PROXY_EXIT_IP_MISMATCH"
	case errors.Is(err, ErrServerIPLeak):
		return "PROXY_SERVER_IP_LEAK"
	case errors.Is(err, ErrForbiddenOutboundHeader):
		return "PROXY_HEADER_LEAK"
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "PROXY_TIMEOUT"
	}
	// x/net's SOCKS authentication errors are unexported plain errors. Match
	// exact leaf messages, never substrings of a URL, remote body or wrapper.
	leaf := err
	for errors.Unwrap(leaf) != nil {
		leaf = errors.Unwrap(leaf)
	}
	switch leaf.Error() {
	case "username/password authentication failed":
		return "PROXY_AUTH_FAILED"
	case "no acceptable authentication methods":
		return "PROXY_AUTH_METHOD_UNSUPPORTED"
	}
	return "PROXY_TRANSPORT_FAILED"
}
