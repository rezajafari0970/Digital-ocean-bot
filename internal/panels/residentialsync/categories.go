package residentialsync

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"os"
	"strings"
)

// Explicitly authorized on 2026-10-09. These include non-ad payload (video,
// downloads and push); this list must not be described as Google-ads-only.
var expandedResidentialCategories = []string{
	"geosite:category-ads", "geosite:category-ads-all",
	"geosite:xiaomi@ads", "geosite:unity@ads", "geosite:huawei@ads", "geosite:gog@ads",
	"geosite:firebase", "geosite:googlefcm", "geosite:google-play", "geosite:youtube",
}

// Older deployed geodata has HUAWEI/GOG but zero @ads entries. Preserve the
// exact attributed domain boundaries from v2fly/domain-list-community commit
// c6adacf29cc4fb05e04651fcf03e00989ef9f797 data/huawei and data/gog.
// Do not expand to entire huawei.com/gog.com or silently refresh global geodata.
var residentialCategoryCompatibility = []string{
	"domain:dt.dbankcloud.ru", "domain:insights-collector.gog.com",
}

// Empty/none/invalid scope preserves the previous policy. Explicit all covers
// existing and future panels; a UUID list stages a bounded production canary.
func ExpandedCategoriesEnabled(panel string) bool {
	scope := strings.TrimSpace(os.Getenv("DOB_RESIDENTIAL_CATEGORIES_PANELS"))
	if scope == "all" {
		return true
	}
	if scope == "" || scope == "none" {
		return false
	}
	selected := false
	for _, item := range strings.Split(scope, ",") {
		id := strings.TrimSpace(item)
		if !residentialperf.UUID.MatchString(id) {
			return false
		}
		selected = selected || strings.EqualFold(id, panel)
	}
	return selected
}

func (p routePolicy) residentialDomains() []string {
	domains := residentialDomains()
	if p.ExpandedCategories {
		domains = append(domains, expandedResidentialCategories...)
		domains = append(domains, residentialCategoryCompatibility...)
	}
	return domains
}

// AllowedResidentialDomains is a copy of the expected deployment contract;
// native acceptance must not infer policy from whatever the remote panel has.
func AllowedResidentialDomains(panel string) []string {
	return (routePolicy{ExpandedCategories: ExpandedCategoriesEnabled(panel)}).residentialDomains()
}
