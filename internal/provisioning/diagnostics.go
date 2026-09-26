package provisioning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"regexp"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/redact"
	"golang.org/x/crypto/ssh"
)

type ErrorClass string

const (
	ClassNetwork        ErrorClass = "network"
	ClassTimeout        ErrorClass = "timeout"
	ClassHandshake      ErrorClass = "handshake"
	ClassAuthentication ErrorClass = "authentication"
	ClassHostKey        ErrorClass = "host_key"
	ClassSession        ErrorClass = "session"
	ClassCommand        ErrorClass = "command"
	ClassConfiguration  ErrorClass = "configuration"
	ClassPackage        ErrorClass = "package_manager"
	ClassDNS            ErrorClass = "dns"
	ClassStorage        ErrorClass = "storage"
	ClassPermission     ErrorClass = "permission"
	ClassService        ErrorClass = "service"
	ClassUnknown        ErrorClass = "unknown"
)

type Diagnostic struct {
	Class       ErrorClass
	Code        string
	Message     string
	Fingerprint string
	ExitCode    *int
	Signal      string
	StdoutTail  string
	StderrTail  string
}

func ClassifyError(err error) Diagnostic {
	if err == nil {
		return Diagnostic{}
	}
	d := Diagnostic{Class: ClassUnknown, Code: "SSH_UNKNOWN", Message: redact.Text(err.Error())}
	if errors.Is(err, ErrHostKeyMismatch) {
		d.Class = ClassHostKey
		d.Code = "SSH_HOST_KEY_MISMATCH"
	} else if errors.Is(err, ErrHostKeyVerifierMissing) {
		d.Class = ClassConfiguration
		d.Code = "SSH_HOST_KEY_VERIFIER_MISSING"
	} else if errors.Is(err, ErrInterruptedUnsafe) {
		d.Class = ClassConfiguration
		d.Code = "WORKER_INTERRUPTED_UNSAFE"
	} else if errors.Is(err, ErrCommandOutcomeUnknown) {
		d.Class = ClassCommand
		d.Code = "COMMAND_OUTCOME_UNKNOWN"
	} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		d.Class = ClassTimeout
		d.Code = "SSH_CONTEXT_TIMEOUT"
	} else {
		var ne net.Error
		var exit *ssh.ExitError
		switch {
		case errors.As(err, &exit):
			d.Class = ClassCommand
			d.Code = "SSH_COMMAND_EXIT"
			code := exit.ExitStatus()
			d.ExitCode = &code
			d.Signal = exit.Signal()
		case errors.As(err, &ne) && ne.Timeout():
			d.Class = ClassTimeout
			d.Code = "SSH_CONNECT_TIMEOUT"
		case errors.Is(err, ErrSSHCommand):
			d.Class = ClassCommand
			d.Code = "SSH_COMMAND_FAILED"
		default:
			m := strings.ToLower(err.Error())
			switch {
			case strings.Contains(m, "unable to authenticate"), strings.Contains(m, "no supported methods remain"):
				d.Class = ClassAuthentication
				d.Code = "SSH_AUTH_FAILED"
			case strings.Contains(m, "host key"):
				d.Class = ClassHostKey
				d.Code = "SSH_HOST_KEY_FAILED"
			case strings.Contains(m, "handshake"):
				d.Class = ClassHandshake
				d.Code = "SSH_HANDSHAKE_FAILED"
			case strings.Contains(m, "connection refused"):
				d.Class = ClassNetwork
				d.Code = "SSH_CONNECTION_REFUSED"
			case strings.Contains(m, "no route to host"):
				d.Class = ClassNetwork
				d.Code = "SSH_NO_ROUTE"
			case strings.Contains(m, "connection reset"):
				d.Class = ClassNetwork
				d.Code = "SSH_CONNECTION_RESET"
			case errors.Is(err, ErrSSHNotReady):
				d.Class = ClassNetwork
				d.Code = "SSH_NOT_READY"
			}
		}
	}
	sum := sha256.Sum256([]byte(string(d.Class) + "|" + d.Code + "|" + normalizeDiagnostic(d.Message)))
	d.Fingerprint = hex.EncodeToString(sum[:8])
	return d
}

var diagIPv4 = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
var diagUUID = regexp.MustCompile(`\b[0-9a-f]{8}-[0-9a-f-]{27,36}\b`)
var diagHex = regexp.MustCompile(`\b0x[0-9a-f]+\b`)
var diagNumber = regexp.MustCompile(`\b\d{2,}\b`)

func normalizeDiagnostic(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = diagIPv4.ReplaceAllString(s, "<ip>")
	s = diagUUID.ReplaceAllString(s, "<uuid>")
	s = diagHex.ReplaceAllString(s, "<hex>")
	s = diagNumber.ReplaceAllString(s, "<n>")
	fields := strings.Fields(s)
	if len(fields) > 24 {
		fields = fields[:24]
	}
	return strings.Join(fields, " ")
}

func tailDiagnostic(s string) string {
	s = redact.Text(strings.ReplaceAll(s, "\x00", ""))
	if len(s) > 4096 {
		s = s[len(s)-4096:]
	}
	return s
}

func ClassifyCommandFailure(err error, res CommandResult) Diagnostic {
	d := ClassifyError(err)
	d.StdoutTail = tailDiagnostic(res.Stdout)
	d.StderrTail = tailDiagnostic(res.Stderr)
	d.ExitCode = res.ExitCode
	d.Signal = res.Signal
	m := strings.ToLower(res.Stderr + "\n" + res.Stdout + "\n" + err.Error())
	switch {
	case strings.EqualFold(res.Signal, "KILL") || strings.Contains(m, "process killed"):
		d.Class = ClassCommand
		d.Code = "PROCESS_KILLED"
	case strings.Contains(m, "could not get lock"), strings.Contains(m, "unable to acquire the dpkg frontend lock"), strings.Contains(m, "is another process using it"):
		d.Class = ClassPackage
		d.Code = "PKG_LOCK_BUSY"
	case strings.Contains(m, "temporary failure resolving"), strings.Contains(m, "could not resolve"), strings.Contains(m, "name or service not known"):
		d.Class = ClassDNS
		d.Code = "DNS_RESOLUTION_FAILED"
	case strings.Contains(m, "no space left on device"):
		d.Class = ClassStorage
		d.Code = "DISK_FULL"
	case strings.Contains(m, "permission denied"), strings.Contains(m, "operation not permitted"):
		d.Class = ClassPermission
		d.Code = "PERMISSION_DENIED"
	case strings.Contains(m, "dpkg was interrupted"):
		d.Class = ClassPackage
		d.Code = "DPKG_INTERRUPTED"
	case strings.Contains(m, "unmet dependencies"), strings.Contains(m, "held broken packages"):
		d.Class = ClassPackage
		d.Code = "PKG_DEPENDENCY_FAILED"
	case strings.Contains(m, "failed to start"), strings.Contains(m, "unit ") && strings.Contains(m, " failed"):
		d.Class = ClassService
		d.Code = "SERVICE_START_FAILED"
	case strings.Contains(m, "connection reset by peer"), strings.Contains(m, "broken pipe"), strings.Contains(m, "unexpected eof"):
		d.Class = ClassNetwork
		d.Code = "SSH_DISCONNECTED"
	}
	sum := sha256.Sum256([]byte(string(d.Class) + "|" + d.Code + "|" + normalizeDiagnostic(d.Message+" "+d.StderrTail)))
	d.Fingerprint = hex.EncodeToString(sum[:8])
	return d
}

func DiagnosticRetryable(d Diagnostic) bool {
	switch d.Code {
	case "PKG_LOCK_BUSY", "DNS_RESOLUTION_FAILED", "DPKG_INTERRUPTED", "SERVICE_START_FAILED", "SSH_DISCONNECTED", "SSH_CONNECTION_REFUSED", "SSH_CONNECT_TIMEOUT", "SSH_NO_ROUTE", "SSH_CONNECTION_RESET", "SSH_NOT_READY", "SSH_CONTEXT_TIMEOUT", "PROCESS_KILLED":
		return true
	case "DISK_FULL", "PERMISSION_DENIED", "SSH_HOST_KEY_FAILED", "COMMAND_OUTCOME_UNKNOWN":
		return false
	default:
		return d.Class != ClassConfiguration
	}
}
