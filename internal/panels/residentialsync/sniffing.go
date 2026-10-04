package residentialsync

import (
	"encoding/json"
	"errors"
)

// A domain route proof is insufficient if actual inbound traffic never exposes
// its hostname. Require installed HTTP/TLS/QUIC sniffing before publishing the
// new routing plan. This does not claim to classify encrypted/opaque hostnames.
func validateAdSniffing(raws []json.RawMessage) error {
	for _, raw := range raws {
		var in struct {
			Protocol string
			Sniffing json.RawMessage
		}
		if json.Unmarshal(raw, &in) != nil {
			return errors.New("invalid inbound sniffing inventory")
		}
		if in.Protocol != "vless" && in.Protocol != "vmess" && in.Protocol != "trojan" && in.Protocol != "shadowsocks" {
			continue
		}
		m, err := decodeObject(in.Sniffing)
		if err != nil {
			return errors.New("advertising routing requires inbound sniffing")
		}
		if m["enabled"] != true || m["metadataOnly"] == true {
			return errors.New("advertising routing requires payload sniffing")
		}
		for _, key := range []string{"domainsExcluded", "ipsExcluded"} {
			if values, ok := m[key].([]any); ok && len(values) > 0 {
				return errors.New("advertising routing cannot exclude sniffing destinations")
			}
		}
		protocols, ok := m["destOverride"].([]any)
		if !ok {
			return errors.New("inbound sniffing protocols missing")
		}
		found := map[string]bool{}
		for _, p := range protocols {
			if s, ok := p.(string); ok {
				found[s] = true
			}
		}
		if !found["http"] || !found["tls"] || !found["quic"] {
			return errors.New("advertising routing requires HTTP TLS and QUIC sniffing")
		}
	}
	return nil
}
