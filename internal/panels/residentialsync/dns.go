package residentialsync

import "github.com/rezajafari0970/Digital-ocean-bot/internal/panels/resolverpolicy"

const dnsTag = "dob-route-dns-query"

func managedDNS() map[string]any {
	return map[string]any{
		"servers":                resolverpolicy.TCPServers(),
		"queryStrategy":          "UseIP",
		"disableCache":           false,
		"disableFallbackIfMatch": true,
		"tag":                    dnsTag,
	}
}
