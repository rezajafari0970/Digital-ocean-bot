package residentialsync

import "github.com/rezajafari0970/Digital-ocean-bot/internal/panels/resolverpolicy"

const dnsTag = "dob-route-dns-query"
const knownDomainPattern = `regexp:^[a-zA-Z0-9_-]+(?:\.[a-zA-Z0-9_-]+)*\.[a-zA-Z][a-zA-Z0-9-]*\.?$`

func managedDNS() map[string]any {
	return map[string]any{
		"servers":                resolverpolicy.Active(),
		"queryStrategy":          "UseIPv4",
		"disableCache":           false,
		"disableFallbackIfMatch": true,
		"tag":                    dnsTag,
	}
}
