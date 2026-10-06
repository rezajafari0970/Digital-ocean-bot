package provisioning

// InstallerPrerequisites returns the immutable steps that must have durable
// success before the first installer placeholder can be entered.
func InstallerPrerequisites(p Plan, readiness bool) ([]string, string, error) {
	steps := append([]ScriptStep(nil), p.Scripts...)
	if len(steps) == 0 {
		steps = LegacyScriptPlan(p).Steps
	}
	if err := validateScriptPlan(steps); err != nil {
		return nil, "", err
	}
	before := []string{"ssh"}
	if readiness {
		before = append(before, "readiness")
	}
	for _, s := range steps {
		if IsInstallerPlaceholder(s) {
			return before, s.Name, nil
		}
		before = append(before, s.Name)
	}
	return nil, "", ErrInvalidPlan
}
