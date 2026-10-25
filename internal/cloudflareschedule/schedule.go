// Package cloudflareschedule defines the shared API and worker rotation limits.
package cloudflareschedule

import "fmt"

const MaxActiveRecords = 8

type Schedule struct {
	Records, Interval, MinLifetime, MaxLifetime, MaxActive int
}

// ValidationError retains the public error code while providing actionable detail.
type ValidationError struct {
	Field          string
	Detail         string
	RequiredActive int
}

func (e *ValidationError) Error() string { return "profile_cloudflare_schedule_invalid" }

func (s Schedule) Validate() error {
	invalid := func(field, detail string) error { return &ValidationError{Field: field, Detail: detail} }
	if s.Records < 1 || s.Records > 2 {
		return invalid("cloudflare_records", "Records per server per interval must be 1 or 2.")
	}
	if s.Interval < 300 {
		return invalid("cloudflare_interval_seconds", "Creation interval must be at least 5 minutes.")
	}
	if s.MinLifetime < 900 || s.MinLifetime > 86400 {
		return invalid("cloudflare_min_lifetime_seconds", "Minimum lifetime must be between 15 minutes and 24 hours.")
	}
	if s.MaxLifetime < s.MinLifetime || s.MaxLifetime > 86400 {
		return invalid("cloudflare_max_lifetime_seconds", "Maximum lifetime must be at least the minimum lifetime and no more than 24 hours.")
	}
	if s.Interval > s.MinLifetime-300 {
		return invalid("cloudflare_interval_seconds", "Creation interval must be at least 5 minutes shorter than the minimum lifetime.")
	}
	// Inputs are bounded above before arithmetic. Keep one spare generation
	// for overlap while expired DNS records are being removed.
	required := s.RequiredActive()
	if required > MaxActiveRecords {
		return &ValidationError{Field: "cloudflare_interval_seconds", RequiredActive: required, Detail: fmt.Sprintf("This schedule needs %d active records per server; the limit is 8. Increase the creation interval, shorten the maximum lifetime, or reduce records per interval.", required)}
	}
	if s.MaxActive < required {
		return &ValidationError{Field: "cloudflare_max_active", RequiredActive: required, Detail: fmt.Sprintf("Maximum active records per server must be at least %d for this schedule (up to 8).", required)}
	}
	if s.MaxActive > MaxActiveRecords {
		return &ValidationError{Field: "cloudflare_max_active", RequiredActive: required, Detail: "Maximum active records per server cannot exceed 8."}
	}
	return nil
}

// RequiredActive returns the overlap allowance for an already validated schedule.
// Call Validate before using this value for an externally supplied schedule.
func (s Schedule) RequiredActive() int {
	return ((s.MaxLifetime+s.Interval-1)/s.Interval + 1) * s.Records
}
