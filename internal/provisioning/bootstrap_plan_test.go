package provisioning

import "testing"

func TestDefaultBootstrapPlanIsIndependentAndReconcilable(t *testing.T) {
	steps := DefaultBootstrapSteps()
	if len(steps) < 5 {
		t.Fatalf("steps=%d", len(steps))
	}
	seen := map[string]bool{}
	for _, s := range steps {
		if seen[s.Name] {
			t.Fatalf("duplicate %s", s.Name)
		}
		seen[s.Name] = true
		if s.MaxAttempts < 1 || s.Timeout <= 0 {
			t.Fatalf("missing policy: %+v", s)
		}
		if s.Execute != "" && s.Name != "bootstrap-verify" && s.Precheck == "" {
			t.Fatalf("state-changing step lacks precheck: %s", s.Name)
		}
		if s.Verify == "" {
			t.Fatalf("step lacks verify: %s", s.Name)
		}
	}
}
func TestDefaultPreInstallerStopsAtPlaceholder(t *testing.T) {
	p := Plan{Scripts: DefaultPreInstallerSteps()}
	if InstallerPlaceholderName(p) != "panel" {
		t.Fatalf("placeholder=%q", InstallerPlaceholderName(p))
	}
	if err := validateScriptPlan(p.Scripts); err != nil {
		t.Fatal(err)
	}
}
