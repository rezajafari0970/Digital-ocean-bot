package sanaei

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

var ErrInvalidInstallerDefinition = errors.New("invalid sanaei installer definition")
var versionRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
var installerSHA256RE = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

type InstallerDefinition struct {
	Version       int
	Release       string
	ScriptURL     string
	ScriptSHA256  string
	SupportedOS   []string
	SupportedArch []string
	MinMemoryMB   int64
	MinDiskMB     int64
	AutoRollback  bool
}

func DefaultInstallerDefinition() InstallerDefinition {
	return InstallerDefinition{SupportedOS: []string{"ubuntu", "debian"}, SupportedArch: []string{"x86_64", "amd64", "arm64", "aarch64"}, MinMemoryMB: 256, MinDiskMB: 1024}
}

func (d InstallerDefinition) Build() (provisioning.InstallerManifest, []provisioning.ScriptStep, error) {
	if d.Version < 1 || d.Release == "" || !versionRE.MatchString(d.Release) || !strings.HasPrefix(d.ScriptURL, "https://") || !installerSHA256RE.MatchString(d.ScriptSHA256) {
		return provisioning.InstallerManifest{}, nil, ErrInvalidInstallerDefinition
	}
	defaults := DefaultInstallerDefinition()
	if len(d.SupportedOS) == 0 {
		d.SupportedOS = defaults.SupportedOS
	}
	if len(d.SupportedArch) == 0 {
		d.SupportedArch = defaults.SupportedArch
	}
	if d.MinMemoryMB == 0 {
		d.MinMemoryMB = defaults.MinMemoryMB
	}
	if d.MinDiskMB == 0 {
		d.MinDiskMB = defaults.MinDiskMB
	}
	artifactName := "sanaei-installer-" + d.Release
	dest := "/var/lib/digital-ocean-bot/installers/" + artifactName + ".sh"
	installName := "sanaei-install-" + d.Release
	verifyName := "sanaei-verify-" + d.Release
	install := provisioning.ScriptStep{Name: installName, Version: 1, Category: "install", Precheck: VerifyCommand(), Execute: fmt.Sprintf("set -euo pipefail; export DEBIAN_FRONTEND=noninteractive; export XUI_NONINTERACTIVE=1; bash %s %s", quote(dest), quote(d.Release)), Verify: VerifyCommand(), Timeout: 15 * time.Minute, MaxAttempts: 3}
	verify := provisioning.ScriptStep{Name: verifyName, Version: 1, Category: "verify", Verify: VerifyCommand(), Timeout: 2 * time.Minute, MaxAttempts: 3}
	manifest := provisioning.InstallerManifest{Name: "sanaei-xui", Version: d.Version, SupportedOS: d.SupportedOS, SupportedArch: d.SupportedArch, MinMemoryMB: d.MinMemoryMB, MinDiskMB: d.MinDiskMB, Artifacts: []provisioning.InstallerArtifact{{Name: artifactName, URL: d.ScriptURL, SHA256: d.ScriptSHA256, Destination: dest}}, InstallScripts: []provisioning.ScriptRef{{Name: installName, Version: 1}}, VerifyScripts: []provisioning.ScriptRef{{Name: verifyName, Version: 1}}, Services: []string{"x-ui"}, RequireRoot: true, RequireDNS: true, RequireHTTPS: true, RequireNoReboot: true, AutoRollback: d.AutoRollback}
	return manifest, []provisioning.ScriptStep{install, verify}, nil
}
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

func RegisterInstaller(ctx context.Context, db *sql.DB, d InstallerDefinition) (provisioning.ResolvedInstaller, error) {
	m, scripts, err := d.Build()
	if err != nil {
		return provisioning.ResolvedInstaller{}, err
	}
	sr := provisioning.ScriptRegistry{DB: db}
	for _, s := range scripts {
		if _, err = sr.Create(ctx, s); err != nil {
			return provisioning.ResolvedInstaller{}, err
		}
	}
	return (provisioning.InstallerRegistry{DB: db, Scripts: sr}).Create(ctx, m)
}
