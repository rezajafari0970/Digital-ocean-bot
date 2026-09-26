package provisioning

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var ErrInstallerNotFound = errors.New("installer not found")
var ErrInstallerVersionConflict = errors.New("installer version conflict")
var ErrInstallerIncompatible = errors.New("installer incompatible with server")
var ErrInvalidArtifact = errors.New("invalid installer artifact")

type InstallerRef struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}
type InstallerArtifact struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	Destination string `json:"destination"`
}
type InstallerManifest struct {
	Name            string              `json:"name"`
	Version         int                 `json:"version"`
	SupportedOS     []string            `json:"supported_os,omitempty"`
	SupportedArch   []string            `json:"supported_arch,omitempty"`
	MinMemoryMB     int64               `json:"min_memory_mb,omitempty"`
	MinDiskMB       int64               `json:"min_disk_mb,omitempty"`
	Artifacts       []InstallerArtifact `json:"artifacts,omitempty"`
	InstallScripts  []ScriptRef         `json:"install_scripts"`
	VerifyScripts   []ScriptRef         `json:"verify_scripts,omitempty"`
	RollbackScripts []ScriptRef         `json:"rollback_scripts,omitempty"`
	Services        []string            `json:"services,omitempty"`
	RequireRoot     bool                `json:"require_root,omitempty"`
	RequireDNS      bool                `json:"require_dns,omitempty"`
	RequireHTTPS    bool                `json:"require_https,omitempty"`
	RequireNoReboot bool                `json:"require_no_reboot,omitempty"`
	AutoRollback    bool                `json:"auto_rollback,omitempty"`
}
type ResolvedInstaller struct {
	RegistryID string            `json:"registry_id"`
	Manifest   InstallerManifest `json:"manifest"`
	SHA256     string            `json:"sha256"`
	Steps      []ScriptStep      `json:"steps"`
	Rollback   []ScriptStep      `json:"rollback"`
}
type InstallerRegistry struct {
	DB      *sql.DB
	Scripts ScriptRegistry
}

func InstallerHash(m InstallerManifest) string {
	raw, _ := json.Marshal(m)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (r InstallerRegistry) Create(ctx context.Context, m InstallerManifest) (ResolvedInstaller, error) {
	out := ResolvedInstaller{Manifest: m}
	if r.DB == nil || m.Name == "" || m.Version <= 0 {
		return out, ErrInvalidPlan
	}
	if err := validateInstallerManifest(m); err != nil {
		return out, err
	}
	raw, _ := json.Marshal(m)
	out.SHA256 = InstallerHash(m)
	err := r.DB.QueryRowContext(ctx, `INSERT INTO installers(name,version,manifest,sha256) VALUES($1,$2,$3,$4) ON CONFLICT(name,version) DO NOTHING RETURNING id::text`, m.Name, m.Version, raw, out.SHA256).Scan(&out.RegistryID)
	if errors.Is(err, sql.ErrNoRows) {
		var existing string
		if e := r.DB.QueryRowContext(ctx, `SELECT id::text,sha256 FROM installers WHERE name=$1 AND version=$2`, m.Name, m.Version).Scan(&out.RegistryID, &existing); e != nil {
			return out, e
		}
		if existing != out.SHA256 {
			return out, ErrInstallerVersionConflict
		}
		return out, nil
	}
	return out, err
}
func (r InstallerRegistry) Resolve(ctx context.Context, ref InstallerRef, ready ReadinessSnapshot) (ResolvedInstaller, error) {
	var out ResolvedInstaller
	var raw []byte
	err := r.DB.QueryRowContext(ctx, `SELECT id::text,manifest,sha256 FROM installers WHERE name=$1 AND version=$2 AND active=true`, ref.Name, ref.Version).Scan(&out.RegistryID, &raw, &out.SHA256)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrInstallerNotFound
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out.Manifest); err != nil {
		return out, err
	}
	if InstallerHash(out.Manifest) != out.SHA256 {
		return out, ErrInstallerVersionConflict
	}
	if err = CheckInstallerCompatibility(out.Manifest, ready); err != nil {
		return out, err
	}
	install, err := r.Scripts.Resolve(ctx, out.Manifest.InstallScripts)
	if err != nil {
		return out, err
	}
	for _, s := range install {
		if s.Execute != "" && (s.Precheck == "" || s.Verify == "") {
			return out, fmt.Errorf("%w: install step %s requires precheck and verify", ErrInvalidPlan, s.Name)
		}
	}
	verify, err := r.Scripts.Resolve(ctx, out.Manifest.VerifyScripts)
	if err != nil {
		return out, err
	}
	for _, s := range verify {
		if s.Execute != "" || s.Verify == "" {
			return out, fmt.Errorf("%w: verify step %s must be verify-only", ErrInvalidPlan, s.Name)
		}
	}
	rollback, err := r.Scripts.Resolve(ctx, out.Manifest.RollbackScripts)
	if err != nil {
		return out, err
	}
	for _, s := range rollback {
		if s.Execute != "" && (s.Precheck == "" || s.Verify == "") {
			return out, fmt.Errorf("%w: rollback step %s requires precheck and verify", ErrInvalidPlan, s.Name)
		}
	}
	artifactSteps, err := ArtifactSteps(out.Manifest.Artifacts)
	if err != nil {
		return out, err
	}
	out.Steps = append(artifactSteps, install...)
	out.Steps = append(out.Steps, verify...)
	for _, svc := range out.Manifest.Services {
		if !installerNameRE.MatchString(svc) {
			return out, ErrInvalidPlan
		}
		out.Steps = append(out.Steps, ScriptStep{Name: "health-service-" + svc, Category: "health", Verify: "systemctl is-active --quiet " + shellQuote(svc), Timeout: time.Minute, MaxAttempts: 3})
	}
	out.Rollback = rollback
	return out, nil
}
func CheckInstallerCompatibility(m InstallerManifest, s ReadinessSnapshot) error {
	if s.Status == "BLOCKED" {
		return fmt.Errorf("%w: readiness blocked", ErrInstallerIncompatible)
	}
	if len(m.SupportedOS) > 0 && !containsFold(m.SupportedOS, s.OSID) {
		return fmt.Errorf("%w: os %s", ErrInstallerIncompatible, s.OSID)
	}
	if len(m.SupportedArch) > 0 && !containsFold(m.SupportedArch, s.Architecture) {
		return fmt.Errorf("%w: arch %s", ErrInstallerIncompatible, s.Architecture)
	}
	if m.MinMemoryMB > 0 && s.MemoryMB < m.MinMemoryMB {
		return fmt.Errorf("%w: memory %d<%d", ErrInstallerIncompatible, s.MemoryMB, m.MinMemoryMB)
	}
	if m.RequireRoot && !s.IsRoot {
		return fmt.Errorf("%w: root required", ErrInstallerIncompatible)
	}
	if m.RequireDNS && !s.DNSOK {
		return fmt.Errorf("%w: dns required", ErrInstallerIncompatible)
	}
	if m.RequireHTTPS && !s.OutboundHTTPSOK {
		return fmt.Errorf("%w: outbound https required", ErrInstallerIncompatible)
	}
	if m.RequireNoReboot && s.RebootRequired {
		return fmt.Errorf("%w: reboot required", ErrInstallerIncompatible)
	}
	if m.MinDiskMB > 0 && s.DiskFreeMB < m.MinDiskMB {
		return fmt.Errorf("%w: disk %d<%d", ErrInstallerIncompatible, s.DiskFreeMB, m.MinDiskMB)
	}
	return nil
}
func validateInstallerManifest(m InstallerManifest) error {
	if m.Name == "" || m.Version <= 0 || len(m.InstallScripts) == 0 {
		return ErrInvalidPlan
	}
	for _, a := range m.Artifacts {
		if _, err := artifactStep(a); err != nil {
			return err
		}
	}
	return nil
}
func containsFold(xs []string, v string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

var installerNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
var sha256RE = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

func ArtifactSteps(xs []InstallerArtifact) ([]ScriptStep, error) {
	out := make([]ScriptStep, 0, len(xs))
	for _, a := range xs {
		s, err := artifactStep(a)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}
func artifactStep(a InstallerArtifact) (ScriptStep, error) {
	if !installerNameRE.MatchString(a.Name) || !strings.HasPrefix(a.URL, "https://") || !sha256RE.MatchString(a.SHA256) || !strings.HasPrefix(a.Destination, "/") {
		return ScriptStep{}, ErrInvalidArtifact
	}
	dest := shellQuote(a.Destination)
	url := shellQuote(a.URL)
	hash := strings.ToLower(a.SHA256)
	check := fmt.Sprintf("test -f %s && printf '%%s  %%s\\n' %s %s | sha256sum -c - >/dev/null 2>&1", dest, shellQuote(hash), dest)
	exec := fmt.Sprintf("set -e; tmp=%s; install -d -m 0755 \"$(dirname \"$tmp\")\"; curl -fL --retry 3 --connect-timeout 10 --max-time 300 %s -o \"$tmp\"; printf '%%s  %%s\\n' %s \"$tmp\" | sha256sum -c -; mv \"$tmp\" %s", shellQuote(a.Destination+".part"), url, shellQuote(hash), dest)
	return ScriptStep{Name: "artifact-" + a.Name, Category: "artifact", Precheck: check, Execute: exec, Verify: check, Timeout: 7 * time.Minute, MaxAttempts: 3}, nil
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
