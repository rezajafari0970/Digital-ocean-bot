package providers

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/geoctx"
	"strings"
)

// RegionCountry resolves physical server location, never account identity or
// residential egress. Explicit provider metadata wins. Legacy catalogs remain
// usable without a provider request on the subscription path.
func RegionCountry(provider, id, name, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return geoctx.NormalizeCountryCode(explicit)
	}
	id = strings.ToLower(strings.TrimSpace(id))
	switch provider {
	case "digitalocean":
		// Exact published datacenter IDs; unknown future regions are not guessed.
		// https://docs.digitalocean.com/platform/regional-availability/
		return digitalOceanCountries[id]
	case "vultr":
		// Our legacy adapter constructed Name as City + ", " + API country.
		if i := strings.LastIndex(name, ","); i >= 0 {
			return geoctx.NormalizeCountryCode(name[i+1:])
		}
	case "linode":
		// Historical geographic IDs precede the modern country-prefixed IDs.
		if code := linodeLegacyCountries[id]; code != "" {
			return code
		}
		// Do not parse Name: Los Angeles, CA is US, not Canada.
		if prefix, suffix, ok := strings.Cut(id, "-"); ok && suffix != "" {
			return geoctx.NormalizeCountryCode(prefix)
		}
	case "upcloud":
		// Public zone IDs include country; uk-lon1 uses the provider's UK alias.
		if prefix, suffix, ok := strings.Cut(id, "-"); ok && suffix != "" {
			return geoctx.NormalizeCountryCode(prefix)
		}
	}
	return ""
}

var digitalOceanCountries = map[string]string{
	"nyc1": "US", "nyc2": "US", "nyc3": "US", "sfo1": "US", "sfo2": "US", "sfo3": "US",
	"ams2": "NL", "ams3": "NL", "sgp1": "SG", "lon1": "GB", "fra1": "DE", "tor1": "CA",
	"blr1": "IN", "syd1": "AU", "atl1": "US", "ric1": "US", "mkc1": "US", "mem1": "US",
}
var linodeLegacyCountries = map[string]string{
	"ap-northeast": "JP", "ap-northeast-1": "JP", "ap-south": "SG", "ap-southeast": "AU",
	"ap-west": "IN", "eu-central": "DE", "eu-west": "GB",
}
