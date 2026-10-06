package serverprotection

import "testing"

func TestGuardianUpgradeScope(t *testing.T) {
	for _, tc := range []struct {
		raw, panel string
		allowed    bool
	}{
		{"", "a", true}, {"none", "a", false}, {"a,b", "a", true},
		{"a,b", "c", false}, {" a, b ", "b", true},
	} {
		c := Controller{UpgradePanels: ParseUpgradePanels(tc.raw)}
		if got := c.upgradeAllowed(tc.panel); got != tc.allowed {
			t.Fatalf("scope %q panel %q: %v", tc.raw, tc.panel, got)
		}
	}
}
