package app

import "testing"

func TestTerminalDeploymentStateForCreateRecovery(t *testing.T) {
	for _, s := range []string{"FAILED", "INSTALL_FAILED", "INSTALL_ROLLED_BACK", "PANEL_COMPLETE", "READY"} {
		if !terminalDeploymentStateForCreateRecovery(s) {
			t.Fatalf("%s must be terminal for stale-create recovery", s)
		}
	}
	for _, s := range []string{"PLANNED", "RESERVED", "CREATING", "WAITING_RESOURCE", "PROVISIONING"} {
		if terminalDeploymentStateForCreateRecovery(s) {
			t.Fatalf("%s must remain recoverable/non-terminal", s)
		}
	}
}
