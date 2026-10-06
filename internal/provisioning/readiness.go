package provisioning

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

var ErrServerReadinessBlocked = errors.New("server readiness blocked")

type ReadinessSnapshot struct {
	Status          string            `json:"status"`
	OSID            string            `json:"os_id"`
	OSVersion       string            `json:"os_version"`
	Architecture    string            `json:"architecture"`
	CPUCount        int               `json:"cpu_count"`
	MemoryMB        int64             `json:"memory_mb"`
	DiskFreeMB      int64             `json:"disk_free_mb"`
	IsRoot          bool              `json:"is_root"`
	PackageManager  string            `json:"package_manager"`
	PackageHealth   string            `json:"package_health"`
	DNSOK           bool              `json:"dns_ok"`
	OutboundHTTPSOK bool              `json:"outbound_https_ok"`
	TimeSync        string            `json:"time_sync"`
	RebootRequired  bool              `json:"reboot_required"`
	Checks          map[string]string `json:"checks"`
}
type ReadinessRecorder interface {
	Readiness(context.Context, string, Target, ReadinessSnapshot, []ReadinessIssue) error
}
type ReadinessCollector struct {
	SSH      StagedCommandRunner
	Recorder ReadinessRecorder
}

func (c ReadinessCollector) Collect(ctx context.Context, runID string, t Target, key []byte, stages StageObserver) (ReadinessSnapshot, error) {
	s := ReadinessSnapshot{Status: "READY", TimeSync: "unknown", Checks: map[string]string{}}
	probeFailure := ""
	blockedProbe := func(code string) (ReadinessSnapshot, error) {
		retryable := readinessSSHRetryable(code)
		issue := ReadinessIssue{Code: code, Severity: "BLOCK", Action: "BLOCK", Detail: "SSH readiness probe failed", State: "OPEN"}
		if retryable {
			issue.Action = "RETRY"
		}
		s.Status = "BLOCKED"
		if c.Recorder != nil {
			if err := c.Recorder.Readiness(ctx, runID, t, s, []ReadinessIssue{issue}); err != nil {
				return s, err
			}
		}
		return s, &ReadinessError{Codes: []string{code}, Retryable: retryable}
	}
	run := func(name, cmd string) string {
		if probeFailure != "" {
			return ""
		}
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		r, err := c.SSH.RunDetailedObserved(cctx, t, key, cmd, stages)
		if err != nil {
			code := readinessProbeCode(err, r)
			s.Checks[name] = "error:" + code
			if readinessSSHRetryable(code) || strings.HasPrefix(code, "SSH_HOST_KEY_") {
				probeFailure = code
			}
			return ""
		}
		v := strings.TrimSpace(r.Stdout)
		s.Checks[name] = tailDiagnostic(v)
		return v
	}
	osraw := run("os", ". /etc/os-release 2>/dev/null; printf '%s|%s' \"$ID\" \"$VERSION_ID\"")
	if code := strings.TrimPrefix(s.Checks["os"], "error:"); strings.HasPrefix(s.Checks["os"], "error:") {
		return blockedProbe(code)
	}
	if p := strings.SplitN(osraw, "|", 2); len(p) == 2 {
		s.OSID = p[0]
		s.OSVersion = p[1]
	}
	s.Architecture = run("architecture", "uname -m")
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	s.CPUCount, _ = strconv.Atoi(run("cpu", "getconf _NPROCESSORS_ONLN 2>/dev/null || nproc"))
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	mem, _ := strconv.ParseInt(run("memory_mb", "awk '/MemTotal/{printf \"%d\",$2/1024}' /proc/meminfo"), 10, 64)
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	s.MemoryMB = mem
	disk, _ := strconv.ParseInt(run("disk_free_mb", "df -Pm / | awk 'NR==2{print $4}'"), 10, 64)
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	s.DiskFreeMB = disk
	s.IsRoot = run("root", "id -u") == "0"
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	_ = run("sudo", `if [ "$(id -u)" = 0 ]; then echo root; elif sudo -n true 2>/dev/null; then echo passwordless; else echo unavailable; fi`)
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	s.PackageManager = run("package_manager", "for x in apt-get dnf yum apk; do command -v \"$x\" >/dev/null 2>&1 && { echo \"$x\"; exit 0; }; done; exit 1")
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	s.PackageHealth = run("package_health", `if command -v dpkg >/dev/null; then out=$(dpkg --audit 2>&1); [ -z "$out" ] && echo ok || { echo "$out"; exit 1; }; else echo unknown; fi`)
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	packageLock := run("package_lock", `if command -v fuser >/dev/null && { fuser /var/lib/dpkg/lock-frontend >/dev/null 2>&1 || fuser /var/lib/dpkg/lock >/dev/null 2>&1; }; then echo busy; else echo free; fi`)
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	s.DNSOK = run("dns", "getent hosts deb.debian.org >/dev/null 2>&1 && echo ok") == "ok"
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	s.OutboundHTTPSOK = run("https", "if command -v curl >/dev/null; then curl -fsSIL --max-time 8 https://deb.debian.org >/dev/null && echo ok; elif command -v wget >/dev/null; then wget -q --spider --timeout=8 https://deb.debian.org && echo ok; else exit 1; fi") == "ok"
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	timeSync := run("time_sync", "if command -v timedatectl >/dev/null; then timedatectl show -p NTPSynchronized --value 2>/dev/null || true; else echo unknown; fi")
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	s.TimeSync = timeSync
	rebootStatus := run("reboot_required", KernelMemoryRebootRequiredCommand())
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	s.RebootRequired = rebootStatus != "no"
	_ = packageLock
	// A lost transport yields missing observations, not proof of non-root,
	// broken packages, or a full disk. Stop before any package remediation.
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	issues := DecideReadiness(s)
	// Only low-risk, idempotent remediation is automatic. We never kill lock
	// holders, change DNS, resize disks, reboot, or alter firewall/network here.
	for i := range issues {
		if issues[i].Action != "AUTO_REMEDIATE" || issues[i].Remediation == "" {
			continue
		}
		remCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		remResult, remErr := c.SSH.RunDetailedObserved(remCtx, t, key, issues[i].Remediation, stages)
		cancel()
		if remErr != nil {
			code := readinessProbeCode(remErr, remResult)
			if readinessSSHRetryable(code) || strings.HasPrefix(code, "SSH_HOST_KEY_") {
				return blockedProbe(code)
			}
			issues[i].State = "FAILED"
			continue
		}
		issues[i].State = "REMEDIATED"
		if issues[i].Code == "PACKAGE_STATE_INCOMPLETE" {
			s.PackageHealth = run("package_health_after_remediation", `out=$(dpkg --audit 2>&1); [ -z "$out" ] && echo ok || { echo "$out"; exit 1; }`)
		}
	}
	if probeFailure != "" {
		return blockedProbe(probeFailure)
	}
	// Re-evaluate after remediation; remediated historical issues remain recorded.
	current := DecideReadiness(s)
	s.Status = readinessStatus(current)
	seen := map[string]bool{}
	for _, i := range issues {
		seen[i.Code] = true
	}
	for _, i := range current {
		if !seen[i.Code] {
			issues = append(issues, i)
		}
	}
	if c.Recorder != nil {
		if err := c.Recorder.Readiness(ctx, runID, t, s, issues); err != nil {
			return s, err
		}
	}
	if s.Status == "BLOCKED" {
		return s, readinessError(current)
	}
	return s, nil
}

// Plain EOF is transport loss for these observational probes (and the
// idempotent dpkg repair). Do not weaken global mutation outcome classification.
func readinessProbeCode(err error, result CommandResult) string {
	code := ClassifyCommandFailure(err, result).Code
	if !strings.HasPrefix(code, "SSH_HOST_KEY_") && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
		return "SSH_DISCONNECTED"
	}
	return code
}

// Only known transient SSH/boot conditions retry; semantic readiness failures
// and host-key/configuration errors retain their existing blocking behavior.
func readinessSSHRetryable(code string) bool {
	switch code {
	case "SSH_CONNECTION_REFUSED", "SSH_CONNECT_TIMEOUT", "SSH_CONTEXT_TIMEOUT", "SSH_NO_ROUTE",
		"SSH_CONNECTION_RESET", "SSH_DISCONNECTED", "SSH_NOT_READY", "SSH_HANDSHAKE_FAILED", "SSH_AUTH_FAILED":
		return true
	default:
		return false
	}
}

func (s SQLStore) Readiness(ctx context.Context, runID string, t Target, r ReadinessSnapshot, issues []ReadinessIssue) error {
	raw, _ := json.Marshal(r.Checks)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var readinessID string
	err = tx.QueryRowContext(ctx, "INSERT INTO server_readiness_snapshots(run_id,account_id,droplet_id,status,os_id,os_version,architecture,cpu_count,memory_mb,disk_free_mb,is_root,package_manager,package_health,dns_ok,outbound_https_ok,time_sync,reboot_required,checks) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING id::text", runID, t.AccountID, t.DropletID, r.Status, r.OSID, r.OSVersion, r.Architecture, r.CPUCount, r.MemoryMB, r.DiskFreeMB, r.IsRoot, r.PackageManager, r.PackageHealth, r.DNSOK, r.OutboundHTTPSOK, r.TimeSync, r.RebootRequired, raw).Scan(&readinessID)
	if err != nil {
		return err
	}
	for _, i := range issues {
		if i.State == "" {
			i.State = "OPEN"
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO server_readiness_issues(readiness_id,run_id,code,severity,action,remediation,state,detail) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, readinessID, runID, i.Code, i.Severity, i.Action, i.Remediation, i.State, i.Detail)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
