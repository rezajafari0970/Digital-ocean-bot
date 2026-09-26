package provisioning

import (
	"fmt"
	"strings"
)

type ReadinessIssue struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Action      string `json:"action"`
	Remediation string `json:"remediation,omitempty"`
	Detail      string `json:"detail,omitempty"`
	State       string `json:"state"`
}

func DecideReadiness(s ReadinessSnapshot) []ReadinessIssue {
	var out []ReadinessIssue
	add := func(code, severity, action, remediation, detail string) {
		out = append(out, ReadinessIssue{Code: code, Severity: severity, Action: action, Remediation: remediation, Detail: detail, State: "OPEN"})
	}
	if !s.IsRoot {
		add("ROOT_REQUIRED", "BLOCK", "BLOCK", "", "SSH user is not root")
	}
	if s.DiskFreeMB < 1024 {
		add("DISK_LOW", "BLOCK", "BLOCK", "", fmt.Sprintf("free_mb=%d", s.DiskFreeMB))
	}
	if s.PackageManager == "" {
		add("PACKAGE_MANAGER_MISSING", "BLOCK", "BLOCK", "", "no supported package manager")
	}
	if s.PackageHealth == "" {
		if s.PackageManager == "apt-get" {
			add("PACKAGE_STATE_INCOMPLETE", "BLOCK", "AUTO_REMEDIATE", "dpkg --configure -a", "dpkg audit reported incomplete state")
		} else {
			add("PACKAGE_HEALTH_FAILED", "BLOCK", "BLOCK", "", "package health check failed")
		}
	}
	if s.Checks["package_lock"] == "busy" {
		add("PACKAGE_LOCK_BUSY", "BLOCK", "RETRY", "", "package manager lock is busy")
	}
	if !s.DNSOK {
		add("DNS_UNAVAILABLE", "BLOCK", "RETRY", "", "DNS resolution failed")
	}
	if !s.OutboundHTTPSOK {
		add("OUTBOUND_HTTPS_UNAVAILABLE", "BLOCK", "RETRY", "", "HTTPS egress failed")
	}
	if s.RebootRequired {
		add("REBOOT_REQUIRED", "WARN", "WARN", "", "server reports reboot required")
	}
	if s.TimeSync == "" || s.TimeSync == "no" {
		add("TIME_NOT_SYNCHRONIZED", "WARN", "WARN", "", "NTP is not synchronized")
	}
	return out
}

func readinessStatus(issues []ReadinessIssue) string {
	status := "READY"
	for _, i := range issues {
		if i.Severity == "BLOCK" {
			return "BLOCKED"
		}
		if i.Severity == "WARN" {
			status = "DEGRADED"
		}
	}
	return status
}

type ReadinessError struct {
	Codes     []string
	Retryable bool
}

func (e *ReadinessError) Error() string {
	return "server readiness blocked: " + strings.Join(e.Codes, ",")
}
func (e *ReadinessError) Unwrap() error { return ErrServerReadinessBlocked }
func readinessError(issues []ReadinessIssue) *ReadinessError {
	e := &ReadinessError{Retryable: true}
	for _, i := range issues {
		if i.Severity != "BLOCK" {
			continue
		}
		e.Codes = append(e.Codes, i.Code)
		if i.Action == "BLOCK" {
			e.Retryable = false
		}
	}
	return e
}
