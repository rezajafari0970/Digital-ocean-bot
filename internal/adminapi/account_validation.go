package adminapi

import "strings"

func validateAccountSettings(x accountWrite, dropletLimit int, provider string) string {
	if x.LifetimeMinSeconds <= 0 || x.LifetimeMaxSeconds <= 0 {
		return "lifetime_must_be_positive"
	}
	if x.LifetimeMinSeconds > x.LifetimeMaxSeconds {
		return "lifetime_from_exceeds_to"
	}
	// Match the persisted lifecycle policy. Never silently rewrite an accepted
	// form value, which otherwise looks like settings disappearing on reopen.
	if x.LifetimeMinSeconds < 1800 || x.LifetimeMaxSeconds < 1800 {
		return "lifetime_minimum_is_30_minutes"
	}
	if x.LifetimeMinSeconds > 604800 {
		return "lifetime_from_maximum_is_7_days"
	}
	if x.BuildSpacingMinutes <= 0 || x.BuildSpacingMaxMinutes <= 0 {
		return "build_spacing_must_be_positive"
	}
	if x.BuildSpacingMinutes > x.BuildSpacingMaxMinutes {
		return "build_spacing_from_exceeds_to"
	}
	if x.DesiredServerCount <= 0 {
		return "desired_servers_must_be_positive"
	}
	if len(x.Regions) == 0 || len(x.Sizes) == 0 || len(x.Images) == 0 {
		return "region_plan_and_image_required"
	}
	if strings.EqualFold(strings.TrimSpace(provider), "digitalocean") && dropletLimit > 0 && x.DesiredServerCount > dropletLimit {
		return "desired_servers_exceeds_droplet_limit"
	}
	if strings.EqualFold(strings.TrimSpace(provider), "digitalocean") {
		allowedImages := map[string]bool{"ubuntu-22-04-x64": true, "ubuntu-24-04-x64": true, "ubuntu-26-04-x64": true}
		for _, image := range x.Images {
			if !allowedImages[strings.TrimSpace(image)] {
				return "unsupported_os_image"
			}
		}
	} else {
		for _, image := range x.Images {
			if strings.TrimSpace(image) == "" {
				return "unsupported_os_image"
			}
		}
	}
	return ""
}
func hasDup(v []string) bool {
	m := map[string]bool{}
	for _, x := range v {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		if m[x] {
			return true
		}
		m[x] = true
	}
	return false
}
