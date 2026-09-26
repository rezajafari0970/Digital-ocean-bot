package provisioning

import (
	"errors"
	"os/exec"
	"testing"
)

func TestInstallerCompatibility(t *testing.T) {
	m := InstallerManifest{Name: "x", Version: 1, SupportedOS: []string{"ubuntu"}, SupportedArch: []string{"x86_64"}, MinMemoryMB: 400, MinDiskMB: 1000, InstallScripts: []ScriptRef{{Name: "install", Version: 1}}}
	good := ReadinessSnapshot{Status: "READY", OSID: "ubuntu", Architecture: "x86_64", MemoryMB: 512, DiskFreeMB: 5000}
	if err := CheckInstallerCompatibility(m, good); err != nil {
		t.Fatal(err)
	}
	bad := good
	bad.Architecture = "arm64"
	if !errors.Is(CheckInstallerCompatibility(m, bad), ErrInstallerIncompatible) {
		t.Fatal("expected arch incompatibility")
	}
	bad = good
	bad.MemoryMB = 128
	if !errors.Is(CheckInstallerCompatibility(m, bad), ErrInstallerIncompatible) {
		t.Fatal("expected memory incompatibility")
	}
}
func TestArtifactStepIntegrityAndShellSyntax(t *testing.T) {
	a := InstallerArtifact{Name: "core", URL: "https://example.com/release.bin", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Destination: "/tmp/release file.bin"}
	s, err := artifactStep(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{s.Precheck, s.Execute, s.Verify} {
		if err := exec.Command("bash", "-n", "-c", cmd).Run(); err != nil {
			t.Fatalf("bad shell: %v\n%s", err, cmd)
		}
	}
	if s.Precheck == "" || s.Verify == "" || s.Category != "artifact" {
		t.Fatalf("%+v", s)
	}
}
func TestArtifactRejectsUnsafeDefinition(t *testing.T) {
	cases := []InstallerArtifact{{Name: "bad name", URL: "https://x", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Destination: "/tmp/x"}, {Name: "x", URL: "http://x", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Destination: "/tmp/x"}, {Name: "x", URL: "https://x", SHA256: "bad", Destination: "/tmp/x"}, {Name: "x", URL: "https://x", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Destination: "relative"}}
	for _, a := range cases {
		if _, err := artifactStep(a); !errors.Is(err, ErrInvalidArtifact) {
			t.Fatalf("accepted %+v", a)
		}
	}
}
func TestInstallerHashDeterministic(t *testing.T) {
	m := InstallerManifest{Name: "x", Version: 1, InstallScripts: []ScriptRef{{Name: "a", Version: 1}}}
	if InstallerHash(m) != InstallerHash(m) || len(InstallerHash(m)) != 64 {
		t.Fatal("bad hash")
	}
}
