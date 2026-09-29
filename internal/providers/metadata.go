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

type Metadata struct {
	Name            string   `json:"name"`
	DisplayName     string   `json:"display_name"`
	CredentialLabel string   `json:"credential_label"`
	Defaults        Defaults `json:"defaults"`
}
