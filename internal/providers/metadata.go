package providers

type ImagePolicy struct {
	Family   string   `json:"family"`
	Versions []string `json:"versions"`
}

type Defaults struct {
	Regions                []string    `json:"regions"`
	Plans                  []string    `json:"plans"`
	Images                 ImagePolicy `json:"images"`
	LifetimeMinMinutes     int         `json:"lifetime_min_minutes"`
	LifetimeMaxMinutes     int         `json:"lifetime_max_minutes"`
	DesiredServers         int         `json:"desired_servers"`
	BuildSpacingMinMinutes int         `json:"build_spacing_min_minutes"`
	BuildSpacingMaxMinutes int         `json:"build_spacing_max_minutes"`
	MaxConcurrent          int         `json:"max_concurrent"`
	FallbackAnyRegion      bool        `json:"fallback_any_region"`
}

type CredentialField struct {
	Advanced bool   `json:"advanced,omitempty"`
	Key      string `json:"key"`
	Label    string `json:"label"`
	Secret   bool   `json:"secret,omitempty"`
	Optional bool   `json:"optional,omitempty"`
	List     bool   `json:"list,omitempty"`
	Default  string `json:"default,omitempty"`
}

type Metadata struct {
	WebsiteURL             string            `json:"website_url,omitempty"`
	AutomaticAuthorization bool              `json:"automatic_authorization,omitempty"`
	CredentialFields       []CredentialField `json:"credential_fields,omitempty"`
	CredentialHelp         string            `json:"credential_help,omitempty"`
	Name                   string            `json:"name"`
	DisplayName            string            `json:"display_name"`
	CredentialLabel        string            `json:"credential_label"`
	Status                 string            `json:"status"`
	Defaults               Defaults          `json:"defaults"`
}
