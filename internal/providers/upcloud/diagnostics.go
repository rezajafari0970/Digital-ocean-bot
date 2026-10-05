package upcloud

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"syscall"
)

var safeDiagnostic = regexp.MustCompile("^[A-Za-z0-9_.-]{1,96}$")

// responseError contains only application-owned codes, never provider bodies.
type responseError struct {
	Code   string
	Status int
}

func (e *responseError) Error() string { return "UpCloud response validation failed: " + e.Code }

type diagnosticError struct {
	error
	code string
}

func (e *diagnosticError) Unwrap() error { return e.error }
func withDiagnostic(err error, code string) error {
	if !safeDiagnostic.MatchString(code) {
		code = "VALIDATION_FAILED"
	}
	return &diagnosticError{error: err, code: code}
}

// Diagnostic returns only bounded codes; it never exposes err.Error(), URLs or credentials.
func Diagnostic(err error) string {
	var d *diagnosticError
	if errors.As(err, &d) {
		return d.code
	}
	var r *responseError
	if errors.As(err, &r) {
		return r.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT"
	}
	if errors.Is(err, context.Canceled) {
		return "CANCELED"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "DNS_FAILED"
	}
	var cert *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	if errors.As(err, &cert) || errors.As(err, &unknown) {
		return "TLS_VERIFICATION_FAILED"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "TIMEOUT"
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return "CONNECTION_RESET"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "CONNECTION_REFUSED"
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "CONNECTION_CLOSED"
	}
	return "REQUEST_FAILED"
}

func apiDiagnostic(status int, code string) string {
	switch code {
	case "AUTHENTICATION_FAILED", "INSUFFICIENT_CREDITS", "ACCOUNT_LOCKED", "ACCOUNT_SUSPENDED", "SERVER_CORE_LIMIT_REACHED", "SERVER_MEMORY_LIMIT_REACHED", "SERVER_IP_LIMIT_REACHED", "PUBLIC_IPV4_LIMIT_REACHED", "HDD_STORAGE_LIMIT_REACHED", "STANDARD_STORAGE_LIMIT_REACHED", "MAXIOPS_STORAGE_LIMIT_REACHED", "SERVER_RESOURCES_UNAVAILABLE", "STORAGE_RESOURCES_UNAVAILABLE", "IP_ADDRESS_RESOURCES_UNAVAILABLE", "SERVER_FORBIDDEN", "STORAGE_FORBIDDEN", "SERVER_NOT_FOUND", "STORAGE_NOT_FOUND", "SERVER_STATE_ILLEGAL", "STORAGE_STATE_ILLEGAL", "STORAGE_ATTACHED", "RATE_LIMITED", "SERVICE_UNAVAILABLE":
		return code
	}
	return fmt.Sprintf("HTTP_%d", status)
}
