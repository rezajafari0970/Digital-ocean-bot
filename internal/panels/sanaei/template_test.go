package sanaei

import (
	"os"
	"strings"
	"testing"
)

func TestInspectTemplateRequiresSQLite(t *testing.T) {
	p := t.TempDir() + "/x-ui.db"
	body := append([]byte("SQLite format 3\x00"), make([]byte, 64)...)
	if err := os.WriteFile(p, body, 0600); err != nil {
		t.Fatal(err)
	}
	x, err := InspectTemplate(p)
	if err != nil {
		t.Fatal(err)
	}
	if x.SHA256 == "" || x.Size != int64(len(body)) {
		t.Fatal("template metadata missing")
	}
}

func TestImportCommandHasRollbackAndChecksum(t *testing.T) {
	cmd := ImportCommand(DefaultDatabasePaths(), strings.Repeat("a", 64))
	for _, needle := range []string{"sha256sum -c -", "systemctl stop x-ui", "cp -a", "systemctl start x-ui", "systemctl is-active x-ui"} {
		if !strings.Contains(cmd, needle) {
			t.Fatalf("missing %s", needle)
		}
	}
}
