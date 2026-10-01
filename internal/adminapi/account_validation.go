package adminapi

import "strings"

func validateAccountSettings(x accountWrite, dropletLimit int, provider string) string {
	if x.LifetimeMinSeconds <= 0 || x.LifetimeMaxSeconds <= 0 {
		return "lifetime_must_be_positive"
	}
	if x.LifetimeMinSeconds > x.LifetimeMaxSeconds {
		return "lifetime_from_exceeds_to"
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
	if !strings.EqualFold(strings.TrimSpace(provider), "vultr") && dropletLimit > 0 && x.DesiredServerCount > dropletLimit {
		return "desired_servers_exceeds_droplet_limit"
	}
	if hasDup(x.Regions) {
		return "duplicate_regions"
	}
	if hasDup(x.Sizes) {
		return "duplicate_plans"
	}
	if hasDup(x.Images) {
		return "duplicate_images"
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
