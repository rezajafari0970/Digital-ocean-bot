package cloudflareschedule_test

import (
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflare"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflareschedule"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"testing"
)

func TestCloudflareScheduleValidation(t *testing.T) {
	base := cloudflareschedule.Schedule{Records: 2, Interval: 1800, MinLifetime: 2700, MaxLifetime: 3600, MaxActive: 6}
	cases := []struct {
		name     string
		change   func(*cloudflareschedule.Schedule)
		field    string
		required int
	}{
		{"screenshot_with_sufficient_capacity", func(s *cloudflareschedule.Schedule) {}, "", 0},
		{"screenshot_with_old_default", func(s *cloudflareschedule.Schedule) { s.MaxActive = 4 }, "cloudflare_max_active", 6},
		{"single_record_defaults", func(s *cloudflareschedule.Schedule) { s.Records = 1; s.MaxActive = 4 }, "", 0},
		{"nondivisible_lifetime", func(s *cloudflareschedule.Schedule) { s.MaxLifetime = 3601; s.MaxActive = 6 }, "cloudflare_max_active", 8},
		{"nondivisible_sufficient", func(s *cloudflareschedule.Schedule) { s.MaxLifetime = 3601; s.MaxActive = 8 }, "", 0},
		{"impossible_capacity", func(s *cloudflareschedule.Schedule) { s.Interval = 300 }, "cloudflare_interval_seconds", 26},
		{"zero_interval", func(s *cloudflareschedule.Schedule) { s.Interval = 0 }, "cloudflare_interval_seconds", 0},
		{"huge_interval_no_overflow", func(s *cloudflareschedule.Schedule) { s.Interval = int(^uint(0) >> 1) }, "cloudflare_interval_seconds", 0},
		{"huge_lifetime_no_overflow", func(s *cloudflareschedule.Schedule) { s.MaxLifetime = int(^uint(0) >> 1) }, "cloudflare_max_lifetime_seconds", 0},
		{"short_minimum", func(s *cloudflareschedule.Schedule) { s.MinLifetime = 899 }, "cloudflare_min_lifetime_seconds", 0},
		{"inverted_lifetime", func(s *cloudflareschedule.Schedule) { s.MinLifetime = 3601 }, "cloudflare_max_lifetime_seconds", 0},
		{"overlap_boundary", func(s *cloudflareschedule.Schedule) { s.MinLifetime = 2100 }, "", 0},
		{"no_overlap", func(s *cloudflareschedule.Schedule) { s.MinLifetime = 2099 }, "cloudflare_interval_seconds", 0},
		{"invalid_count", func(s *cloudflareschedule.Schedule) { s.Records = 3 }, "cloudflare_records", 0},
		{"above_capacity_limit", func(s *cloudflareschedule.Schedule) { s.MaxActive = 9 }, "cloudflare_max_active", 6},
	}
	const account = "11111111-1111-4111-8111-111111111111"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			tc.change(&s)
			p := cloudflare.Policy{Account: account, Records: s.Records, Interval: s.Interval, MinLifetime: s.MinLifetime, MaxLifetime: s.MaxLifetime, MaxActive: s.MaxActive}
			spec := configdriver.Spec{Protocol: "vless", Network: "ws", Security: "tls", Port: 2083, Path: "/cdn", CloudflareAccount: account, CloudflareRecords: s.Records, CloudflareInterval: s.Interval, CloudflareMinLifetime: s.MinLifetime, CloudflareMaxLifetime: s.MaxLifetime, CloudflareMaxActive: s.MaxActive}
			for name, err := range map[string]error{"shared": s.Validate(), "worker": p.Validate(), "driver": configdriver.ValidateCloudflare(spec)} {
				if tc.field == "" {
					if err != nil {
						t.Fatalf("%s: %v", name, err)
					}
					continue
				}
				var detail *cloudflareschedule.ValidationError
				if !errors.As(err, &detail) || detail.Field != tc.field || detail.RequiredActive != tc.required || detail.Detail == "" || err.Error() != "profile_cloudflare_schedule_invalid" {
					t.Fatalf("%s: wrong validation detail: %#v", name, err)
				}
			}
		})
	}
}
