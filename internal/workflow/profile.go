package workflow

import (
	"bytes"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"time"
)

type ProviderKeyRef string

func (r *ProviderKeyRef) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		*r = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*r = ProviderKeyRef(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*r = ProviderKeyRef(n.String())
	return nil
}
func (r ProviderKeyRef) MarshalJSON() ([]byte, error) { return json.Marshal(string(r)) }
func (r ProviderKeyRef) String() string               { return string(r) }

type ProfileSnapshot struct {
	Name               string                     `json:"name"`
	Region             string                     `json:"region"`
	Regions            []string                   `json:"regions,omitempty"`
	Size               string                     `json:"size"`
	Image              string                     `json:"image"`
	Lifetime           time.Duration              `json:"lifetime"`
	SSHUser            string                     `json:"ssh_user"`
	SSHKeySecretRef    string                     `json:"ssh_key_secret_ref"`
	SSHProviderKeyID   ProviderKeyRef             `json:"ssh_provider_key_id,omitempty"`
	SSHAuthorizedKey   string                     `json:"ssh_authorized_key,omitempty"`
	SSHKeyFingerprint  string                     `json:"ssh_key_fingerprint,omitempty"`
	InstallerURL       string                     `json:"installer_url"`
	InstallerRef       *provisioning.InstallerRef `json:"installer_ref,omitempty"`
	InstallScriptRefs  []provisioning.ScriptRef   `json:"install_script_refs,omitempty"`
	InstallSteps       []provisioning.ScriptStep  `json:"install_steps,omitempty"`
	DatabaseTemplateID string                     `json:"database_template_id"`
	InboundID          int                        `json:"inbound_id"`
	ClientCount        int                        `json:"client_count"`
	EmailPrefix        string                     `json:"email_prefix"`
}

type PersistentProfile struct {
	ID        string
	AccountID string
	Name      string
	Version   int
	Config    ProfileSnapshot
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
