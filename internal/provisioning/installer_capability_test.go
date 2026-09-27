package provisioning

import "testing"

func TestInstallerCapabilitiesValidated(t *testing.T) {
	good := InstallerManifest{Name: "x", Version: 1, InstallScripts: []ScriptRef{{Name: "s", Version: 1}}, Capabilities: []string{"xui_database", "xui_panel"}}
	if err := validateInstallerManifest(good); err != nil {
		t.Fatal(err)
	}
	for _, caps := range [][]string{{"typo"}, {"xui_database", "xui_database"}} {
		x := good
		x.Capabilities = caps
		if validateInstallerManifest(x) == nil {
			t.Fatalf("accepted %v", caps)
		}
	}
}
