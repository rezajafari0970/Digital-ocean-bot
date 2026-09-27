package sanaei

import (
	"bytes"
	"strings"
	"testing"
)

func TestPanelDefaultsDeterministicAndScoped(t *testing.T) {
	u, p, path, ref := panelDefaults("12345678-1234-1234-1234-123456789abc")
	if u != "dob_123456781234" {
		t.Fatalf("username=%q", u)
	}
	if p != 2053 {
		t.Fatalf("port=%d", p)
	}
	if path != "/123456781234/" {
		t.Fatalf("path=%q", path)
	}
	if ref != "xui-panel-12345678-1234-1234-1234-123456789abc" {
		t.Fatalf("ref=%q", ref)
	}
}
func TestPanelPasswordsAreRandomURLSafeAndNotReused(t *testing.T) {
	a, e := NewPanelPassword()
	if e != nil {
		t.Fatal(e)
	}
	b, e := NewPanelPassword()
	if e != nil {
		t.Fatal(e)
	}
	if len(a) < 32 || len(b) < 32 {
		t.Fatalf("short password")
	}
	if bytes.Equal(a, b) {
		t.Fatal("password reused")
	}
	if strings.ContainsAny(string(a), " \t\r\n/'\"$") {
		t.Fatalf("unsafe password alphabet")
	}
}
func TestPanelConfigureCommandDoesNotContainCredential(t *testing.T) {
	secret := "DO-NOT-LEAK-SECRET"
	cmd := panelConfigureCommand("/root/.dob-xui-panel-test.env")
	if strings.Contains(cmd, secret) {
		t.Fatal("secret leaked into command")
	}
	for _, want := range []string{"$XUI_USER", "$XUI_PASS", "$XUI_PORT", "$XUI_PATH", "systemctl restart x-ui", "systemctl is-active x-ui", "trap 'rm -f"} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("missing %q", want)
		}
	}
}
