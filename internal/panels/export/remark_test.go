package export

import (
	"net/url"
	"strings"
	"testing"
)

func TestCountryRemarkPreservesConnectionBytes(t *testing.T) {
	for _, head := range []string{
		"vless://uuid@[2001:db8::1]:443?type=tcp&security=reality&x=a+b&x=a%20b",
		"vless://uuid@host.test:443?type=xhttp&path=%2f%2F&extra=%7B%22a%22%3A%22b%22%7D&flow=xtls-rprx-vision-udp443",
	} {
		for _, old := range []string{"", "#dob%3Aglobal-reality-u-id", "#wrong#fragment"} {
			got := WithRemark(head+old, "🇩🇪 Germany")
			if strings.SplitN(got, "#", 2)[0] != head || strings.Contains(got, "+") && !strings.Contains(head, "+") {
				t.Fatal("connection bytes changed")
			}
			u, e := url.Parse(got)
			if e != nil || u.Fragment != "🇩🇪 Germany" || strings.Contains(strings.SplitN(got, "#", 2)[1], "+") {
				t.Fatal("fragment encoding")
			}
			if WithRemark(got, "🇩🇪 Germany") != got {
				t.Fatal("not idempotent")
			}
		}
	}
}
