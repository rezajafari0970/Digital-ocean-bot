package export

import (
	"strings"
	"testing"
)

func TestOutputOptionsPreserveWireBytes(t *testing.T) {
	base := "vless://id@[2001:db8::1]:443?type=xhttp&path=%2Fone%2ftwo%2F&host=example.org&flow=xtls-rprx-vision-udp443&encryption=key%2Bvalue"
	extras := "&extra=%7B%22number%22%3A1%7D&extr%61=%7B%7D&fm=%7B%7D&finalmask=%7B%7D"
	browser := "&browserDialerMode=WebView&browserDialer=OkHttp&browser_dialer=WebView"
	tail := "&literal=a+b&encoded=a%20b#label%20a+b"
	for _, opt := range []OutputOptions{{}, {IncludeXHTTPExtra: true}, {IncludeFinalMask: true}, {IncludeXHTTPExtra: true, IncludeFinalMask: true}} {
		got, err := ApplyOutputOptions(base+extras+browser+tail, opt)
		expected := base
		if opt.IncludeXHTTPExtra {
			expected += "&extra=%7B%22number%22%3A1%7D&extr%61=%7B%7D"
		}
		if opt.IncludeFinalMask {
			expected += "&fm=%7B%7D&finalmask=%7B%7D"
		}
		expected += tail
		if err != nil || got != expected {
			t.Fatalf("%+v: %q %v", opt, got, err)
		}
	}
	for _, uri := range []string{"vless://id@host:443#test", "vless://id@host:443?", "vless://id@host:443?type=tcp&security=reality#two"} {
		got, err := ApplyOutputOptions(uri, OutputOptions{})
		if err != nil || got != uri {
			t.Fatal(got, err)
		}
	}
}
func TestOutputOptionsRejectMalformedComponents(t *testing.T) {
	for _, raw := range []string{"extra=%zz", "%xx=value", "=value", "extra", "extra={}&", "extra={}&&fm={}"} {
		got, err := ApplyOutputOptions("vless://id@host:443?"+raw, OutputOptions{})
		if err == nil || got != "" {
			t.Fatal("malformed accepted", raw, got)
		}
	}
	for _, uri := range []string{"https://id@host/?extra={}", "vless://host:443?extra={}", "garbage"} {
		if got, e := ApplyOutputOptions(uri, OutputOptions{}); e == nil || strings.HasPrefix(got, "vless://") {
			t.Fatal(uri, got, e)
		}
	}
}
