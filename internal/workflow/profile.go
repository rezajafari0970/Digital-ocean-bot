package workflow

import "time"

type ProfileSnapshot struct {
	Name               string        `json:"name"`
	Region             string        `json:"region"`
	Regions            []string      `json:"regions,omitempty"`
	Size               string        `json:"size"`
	Image              string        `json:"image"`
	Lifetime           time.Duration `json:"lifetime"`
	SSHUser            string        `json:"ssh_user"`
	SSHKeySecretRef    string        `json:"ssh_key_secret_ref"`
	InstallerURL       string        `json:"installer_url"`
	DatabaseTemplateID string        `json:"database_template_id"`
	InboundID          int           `json:"inbound_id"`
	ClientCount        int           `json:"client_count"`
	EmailPrefix        string        `json:"email_prefix"`
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
