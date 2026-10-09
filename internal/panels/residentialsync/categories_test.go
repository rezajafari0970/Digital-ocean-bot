package residentialsync

import (
	"reflect"
	"strings"
	"testing"
)

func TestCategoryScopeAndExactContract(t *testing.T) {
	const panel = "11111111-1111-4111-8111-111111111111"
	for _, tc := range []struct {
		scope string
		want  bool
	}{{"", false}, {"none", false}, {"all", true}, {panel, true}, {panel + ",bad", false}, {"22222222-2222-4222-8222-222222222222", false}} {
		t.Setenv("DOB_RESIDENTIAL_CATEGORIES_PANELS", tc.scope)
		if got := ExpandedCategoriesEnabled(panel); got != tc.want {
			t.Fatalf("%q: %t", tc.scope, got)
		}
	}
	expected := []string{"geosite:google@ads", "domain:browserleaks.com", "geosite:category-ads", "geosite:category-ads-all", "geosite:xiaomi@ads", "geosite:unity@ads", "geosite:huawei@ads", "geosite:gog@ads", "geosite:firebase", "geosite:googlefcm", "geosite:google-play", "geosite:youtube", "domain:dt.dbankcloud.ru", "domain:insights-collector.gog.com"}
	t.Setenv("DOB_RESIDENTIAL_CATEGORIES_PANELS", "all")
	got := AllowedResidentialDomains(panel)
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("unauthorized or missing scope: %v", got)
	}
	got[0] = "geosite:google"
	if !reflect.DeepEqual(AllowedResidentialDomains(panel), expected) {
		t.Fatal("caller mutated contract")
	}
}

func TestExpandedCategoryPoolAndRollback(t *testing.T) {
	p := routePolicy{StrictAllowlist: true, ExpandedCategories: true, StableFingerprint: true, PoolEnabled: true, Harden: true, AdsOnly: true, Residential: true, Configured: 1, Proxies: []rp{{Type: "socks5", Host: "example.test", Port: 1080, Tag: "residential-ads-test"}}}
	next, e := buildSettings(baseSettings(), nil, []string{"in"}, p)
	if e != nil {
		t.Fatal(e)
	}
	matched := 0
	for _, v := range next["routing"].(map[string]any)["rules"].([]any) {
		rule := v.(map[string]any)
		tag, _ := rule["ruleTag"].(string)
		if strings.HasPrefix(tag, "dob-route-residential-ads-") || strings.HasPrefix(tag, poolPrefix) && strings.HasSuffix(tag, "-ads") {
			if !reflect.DeepEqual(rule["domain"], p.residentialDomains()) {
				t.Fatalf("scope differs in %s", tag)
			}
			matched++
		}
	}
	if matched != 6 {
		t.Fatalf("expected outer TCP/UDP and all four inner scopes, got %d", matched)
	}
	second, e := buildSettings(next, nil, []string{"in"}, p)
	if e != nil || settingsHash(second) != settingsHash(next) {
		t.Fatal("expanded plan not stable", e)
	}
	p.ExpandedCategories = false
	restored, e := buildSettings(next, nil, []string{"in"}, p)
	if e != nil {
		t.Fatal(e)
	}
	original, e := buildSettings(baseSettings(), nil, []string{"in"}, p)
	if e != nil {
		t.Fatal(e)
	}
	if settingsHash(restored) != settingsHash(original) {
		t.Fatal("scoped rollback not exact")
	}
}
