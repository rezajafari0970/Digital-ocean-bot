package adminapi

import "testing"

func validAccountWriteForCapacity() accountWrite {
	return accountWrite{
		LifetimeMinSeconds:     3600,
		LifetimeMaxSeconds:     7200,
		BuildSpacingMinutes:    5,
		BuildSpacingMaxMinutes: 10,
		DesiredServerCount:     15,
		Regions:                []string{"ewr"},
		Sizes:                  []string{"vc2-1c-1gb"},
		Images:                 []string{"2284"},
	}
}

func TestVultrDesiredMayExceedLastKnownProviderCeiling(t *testing.T) {
	x := validAccountWriteForCapacity()
	if got := validateAccountSettings(x, 10, "vultr"); got != "" {
		t.Fatalf("Vultr desired must remain user-controlled, got %q", got)
	}
}

func TestDigitalOceanDesiredStillRespectsExactProviderLimit(t *testing.T) {
	x := validAccountWriteForCapacity()
	x.Images = []string{"ubuntu-24-04-x64"}
	if got := validateAccountSettings(x, 10, "digitalocean"); got != "desired_servers_exceeds_droplet_limit" {
		t.Fatalf("got %q", got)
	}
}

func TestDuplicateDeploymentPreferencesAreAllowed(t *testing.T) {
	x := validAccountWriteForCapacity()
	x.DesiredServerCount = 5
	x.Regions = []string{"ewr", "ewr", "ewr"}
	x.Sizes = []string{"vc2-1c-1gb", "vc2-1c-1gb", "vc2-1c-1gb"}
	x.Images = []string{"2284", "2284", "2284"}
	if got := validateAccountSettings(x, 10, "vultr"); got != "" {
		t.Fatalf("Vultr duplicate preferences must be allowed, got %q", got)
	}
	x.Images = []string{"ubuntu-24-04-x64", "ubuntu-24-04-x64", "ubuntu-24-04-x64"}
	if got := validateAccountSettings(x, 10, "digitalocean"); got != "" {
		t.Fatalf("DigitalOcean duplicate preferences must be allowed, got %q", got)
	}
}

func TestDuplicatePreferencesDoNotBypassImageValidation(t *testing.T) {
	x := validAccountWriteForCapacity()
	x.DesiredServerCount = 5
	x.Images = []string{"not-supported", "not-supported"}
	if got := validateAccountSettings(x, 10, "digitalocean"); got != "unsupported_os_image" {
		t.Fatalf("got %q", got)
	}
}
