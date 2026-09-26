package provisioning

import "time"

type ScriptStep struct {
	RegistryID  string        `json:"registry_id,omitempty"`
	Version     int           `json:"version,omitempty"`
	SHA256      string        `json:"sha256,omitempty"`
	Name        string        `json:"name"`
	Category    string        `json:"category"`
	Precheck    string        `json:"precheck,omitempty"`
	Execute     string        `json:"execute,omitempty"`
	Verify      string        `json:"verify,omitempty"`
	Timeout     time.Duration `json:"timeout"`
	MaxAttempts int           `json:"max_attempts"`
}

type ScriptPlan struct{ Steps []ScriptStep }

func validateScriptPlan(steps []ScriptStep) error {
	seen := map[string]bool{"ssh": true, "done": true}
	for i := range steps {
		s := &steps[i]
		if s.Name == "" || seen[s.Name] || s.Execute == "" && s.Verify == "" {
			return ErrInvalidPlan
		}
		seen[s.Name] = true
		if s.Category == "" {
			s.Category = "script"
		}
		if s.MaxAttempts <= 0 {
			s.MaxAttempts = 3
		}
		if s.Timeout <= 0 {
			s.Timeout = 10 * time.Minute
		}
	}
	return nil
}

func legacyScriptStep(name, command string, max int) ScriptStep {
	timeout := 10 * time.Minute
	if name == "panel" {
		timeout = 15 * time.Minute
	} else if name == "verify" {
		timeout = 5 * time.Minute
	}
	return ScriptStep{Name: name, Category: name, Execute: command, MaxAttempts: max, Timeout: timeout}
}

func LegacyScriptPlan(p Plan) ScriptPlan {
	return ScriptPlan{Steps: []ScriptStep{
		{Name: "bootstrap", Category: "bootstrap", Execute: p.Bootstrap, MaxAttempts: 5, Timeout: 10 * time.Minute},
		{Name: "panel", Category: "install", Execute: p.InstallPanel, MaxAttempts: 5, Timeout: 15 * time.Minute},
		{Name: "verify", Category: "verify", Execute: p.Verify, MaxAttempts: 8, Timeout: 5 * time.Minute},
	}}
}
