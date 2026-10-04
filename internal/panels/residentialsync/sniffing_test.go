package residentialsync

import (
	"encoding/json"
	"testing"
)

func TestAdRoutingRequiresActualInboundSniffing(t *testing.T) {
	for _, body := range []string{
		`{"enabled":true,"destOverride":["http","tls","quic"],"domainsExcluded":["adservice.google.com"]}`,
		`{"enabled":true,"destOverride":["http","tls","quic"],"ipsExcluded":["0.0.0.0/0"]}`,
		`{}`, `{"enabled":false,"destOverride":["http","tls","quic"]}`,
		`{"enabled":true,"metadataOnly":true,"destOverride":["http","tls","quic"]}`,
		`{"enabled":true,"destOverride":["http","tls"]}`,
	} {
		raw := json.RawMessage(`{"protocol":"vless","sniffing":` + body + `}`)
		if validateAdSniffing([]json.RawMessage{raw}) == nil {
			t.Fatal("unverifiable sniffing accepted")
		}
	}
	if err := validateAdSniffing([]json.RawMessage{json.RawMessage(`{"protocol":"vless","sniffing":{"enabled":true,"metadataOnly":false,"destOverride":["http","tls","quic"]}}`)}); err != nil {
		t.Fatal(err)
	}
}
func TestCanaryScopeDefaultsToCompletePolicy(t *testing.T) {
	t.Setenv("DOB_RESIDENTIAL_ADS_ONLY_PANELS", "a, b")
	if !adsOnlyPanel("a") || !adsOnlyPanel("b") || adsOnlyPanel("c") {
		t.Fatal("canary scope escaped")
	}
	t.Setenv("DOB_RESIDENTIAL_ADS_ONLY_PANELS", "")
	if !adsOnlyPanel("c") {
		t.Fatal("rollout did not cover fleet")
	}
}
