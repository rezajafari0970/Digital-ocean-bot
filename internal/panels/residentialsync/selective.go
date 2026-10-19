package residentialsync

// GoogleAdsSelectiveEnabled stages the operator-authorized 2026-10-19 policy.
// Empty/none/invalid keeps the previous policy; all includes future panels.
func GoogleAdsSelectiveEnabled(panel string) bool {
	return relayScopeEnabled("DOB_RESIDENTIAL_GOOGLE_ADS_SELECTIVE_PANELS", panel)
}

const GoogleAdsRelayProbeURL = "https://pagead2.googlesyndication.com/pagead/js/adsbygoogle.js"

// Public client fallthrough is direct; internal pool/relay inlets remain
// category restricted so they can never become an unrestricted proxy.
func (p routePolicy) protectedTransport() bool { return p.StrictAllowlist || p.GoogleAdsSelective }

func (p routePolicy) relayProbeURL() string {
	if p.GoogleAdsSelective {
		return GoogleAdsRelayProbeURL
	}
	return RelayProbeURL
}
