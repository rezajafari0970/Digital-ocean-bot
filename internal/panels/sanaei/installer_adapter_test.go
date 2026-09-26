package sanaei

import (
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

func TestInstallerAdapterBuildsImmutableCoreDefinition(t *testing.T) {
	d := InstallerDefinition{Version: 1, Release: "2.6.0", ScriptURL: "https://example.com/install.sh", ScriptSHA256: strings.Repeat("a", 64)}
	m, scripts, err := d.Build()
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "sanaei-xui" || m.Version != 1 || len(m.Artifacts) != 1 || len(m.InstallScripts) != 1 || len(m.VerifyScripts) != 1 {
		t.Fatalf("%+v", m)
	}
	if m.Artifacts[0].SHA256 != strings.Repeat("a", 64) || m.Artifacts[0].Destination == "" {
		t.Fatal("artifact not pinned")
	}
	if len(scripts) != 2 {
		t.Fatalf("scripts=%d", len(scripts))
	}
	for _, s := range scripts {
		if s.Category == "install" && (s.Precheck == "" || s.Execute == "" || s.Verify == "") {
			t.Fatalf("unsafe install step %+v", s)
		}
	}
}
func TestInstallerAdapterRejectsUnpinnedOrInsecureArtifact(t *testing.T) {
	cases := []InstallerDefinition{{Version: 1, Release: "v1", ScriptURL: "http://example.com/x", ScriptSHA256: strings.Repeat("a", 64)}, {Version: 1, Release: "v1", ScriptURL: "https://example.com/x", ScriptSHA256: "bad"}, {Version: 1, Release: "bad release", ScriptURL: "https://example.com/x", ScriptSHA256: strings.Repeat("a", 64)}}
	for _, d := range cases {
		if _, _, err := d.Build(); err == nil {
			t.Fatalf("accepted %+v", d)
		}
	}
}
func TestInstallerAdapterCompatibilityDefaults(t *testing.T) {
	d := InstallerDefinition{Version: 1, Release: "v1", ScriptURL: "https://example.com/x", ScriptSHA256: strings.Repeat("a", 64)}
	m, _, err := d.Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := provisioning.CheckInstallerCompatibility(m, provisioning.ReadinessSnapshot{Status: "READY", OSID: "ubuntu", Architecture: "x86_64", MemoryMB: 512, DiskFreeMB: 2048}); err != nil {
		t.Fatal(err)
	}
	if err := provisioning.CheckInstallerCompatibility(m, provisioning.ReadinessSnapshot{Status: "READY", OSID: "alpine", Architecture: "x86_64", MemoryMB: 512, DiskFreeMB: 2048}); err == nil {
		t.Fatal("expected incompatible OS")
	}
}
func TestLegacyInstallerCommandDoesNotBecomeAdapterSource(t *testing.T) {
	d := InstallerDefinition{Version: 1, Release: "v1", ScriptURL: "https://example.com/x", ScriptSHA256: strings.Repeat("a", 64)}
	m, scripts, err := d.Build()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(scripts[0].Execute, "XUI_NONINTERACTIVE=1") || !strings.Contains(scripts[0].Execute, "'v1'") {
		t.Fatal("release must be explicit and noninteractive")
	}
	if strings.Contains(scripts[0].Execute, "curl ") {
		t.Fatal("adapter install must execute verified artifact, not fetch network content")
	}
	if !strings.HasPrefix(m.Artifacts[0].URL, "https://") {
		t.Fatal("artifact must use https")
	}
}
