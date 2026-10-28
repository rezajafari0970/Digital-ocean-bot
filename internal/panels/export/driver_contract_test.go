package export_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/export"
)

// Publication must accept every public native driver contract, including the
// TCP HTTP variant that used to break the whole live output bundle.
func TestNativeDriverPublicationContract(t *testing.T) {
	r := configdriver.Builtins()
	c, err := configdriver.NewCredentials("profile", "panel", []string{"DIRECT"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, cap := range r.Catalog() {
		if !cap.Public {
			continue
		}
		for _, header := range []string{"", "http"} {
			if header != "" && (cap.Network != "tcp" || cap.Security == "reality") {
				continue
			}
			t.Run(cap.ID+"/"+header, func(t *testing.T) {
				s := configdriver.Spec{Protocol: cap.Protocol, Network: cap.Network, Security: cap.Security, Port: 15001, HeaderType: header}
				if cap.Security == "reality" {
					s.SNI = "example.com"
				}
				if cap.Network == "ws" || cap.Network == "xhttp" || header == "http" {
					s.Host = "example.com"
					s.Path = "/native%2Fpath"
				}
				if cap.Network == "grpc" {
					s.Path = "native_service"
				}
				uri, err := r.URI(s, "192.0.2.1", "native fixture", c, c.Clients[0])
				if err != nil {
					t.Fatal(err)
				}
				got, err := export.ApplyOutputOptions(uri, export.OutputOptions{IndependentProfile: true})
				if err != nil || got != uri {
					t.Fatalf("native contract rejected or rewritten: %v", err)
				}
			})
		}
	}
}

func TestTrojanHTTPPublicationRejectsInvalidTransport(t *testing.T) {
	base := "trojan://password@192.0.2.1:15001?security=none&type=tcp&headerType=http&host=example.com&path=%2F"
	for _, change := range []string{"headerType=unknown", "type=ws", "host=", "path=%2F%2Fevil.example", "path=%2F%0D%0AHost%3Aevil", "path=%2F%3Fx%3D1", "headerType=http&headerType=none"} {
		u, _ := url.Parse(base)
		key, _, _ := strings.Cut(change, "=")
		q := u.Query()
		q.Del(key)
		u.RawQuery = q.Encode() + "&" + change
		if _, err := export.ApplyOutputOptions(u.String(), export.OutputOptions{IndependentProfile: true}); err == nil {
			t.Fatalf("accepted invalid transport %s", key)
		}
	}
	if _, err := export.ApplyOutputOptions(base, export.OutputOptions{}); err == nil {
		t.Fatal("legacy policy widened")
	}
}
