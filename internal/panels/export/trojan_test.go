package export

import (
	"net/url"
	"strings"
	"testing"
)

func TestTrojanTCPExportPreservesPasswordAndCountry(t *testing.T) {
	for _, host := range []string{"192.0.2.1", "2001:db8::1"} {
		password := strings.Repeat("a", 32) + "+/@:#?"
		uri, e := TrojanTCPURI(TrojanTCP{Host: host, Port: 8443, Password: password, Remark: "🇩🇪 Germany"})
		if e != nil {
			t.Fatal(e)
		}
		u, e := url.Parse(uri)
		if e != nil {
			t.Fatal(e)
		}
		if u.User.Username() != password || u.Hostname() != host || u.Port() != "8443" || u.Fragment != "🇩🇪 Germany" || u.Query().Get("security") != "tls" || u.Query().Get("type") != "tcp" {
			t.Fatal("URI round trip mismatch")
		}
		for _, opts := range []OutputOptions{{}, {IncludeXHTTPExtra: true, IncludeFinalMask: true}} {
			got, e := ApplyOutputOptions(uri, opts)
			if e != nil || got != uri {
				t.Fatal("VLESS output options changed Trojan", e)
			}
		}
		for _, bad := range []string{"allowInsecure=1", "flow=xtls-rprx-vision", "sni=unrelated.example", "extra=%7B%7D"} {
			if _, e := ApplyOutputOptions(strings.Replace(uri, "type=tcp", "type=tcp&"+bad, 1), OutputOptions{}); e == nil {
				t.Fatal("invalid Trojan option accepted")
			}
		}
	}
}
