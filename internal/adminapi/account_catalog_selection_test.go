package adminapi

import (
	"os/exec"
	"testing"
)

func TestAccountCatalogJavaScript(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node unavailable")
	}
	out, err := exec.Command("node", "../../web/tests/account-catalog.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}
