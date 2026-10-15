package export

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
)

// This models the legacy core's singular-only FragmentMask fields after the
// same URI-component decode used by v2rayNG. Unknown plural fields are ignored.
func TestClientMaskLegacySchemaThroughURI(t *testing.T) {
	c := transportprofile.Recommended()
	c.Host = "aparat.com"
	c.Path, _ = transportprofile.NewPath(c.PathBytes)
	_, enc, err := transportprofile.EncryptionPair(base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32))), "legacy-consumer")
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"both", "client", "server", "disabled"} {
		t.Run(scope, func(t *testing.T) {
			cfg := c
			cfg.FinalMaskScope = scope
			if scope == "disabled" {
				cfg.FinalMaskScope = "both"
				cfg.FinalMaskEnabled = false
			}
			uri, err := VLESSRealityURI(VLESSReality{Transport: cfg, UUID: "b530c072-21c6-405d-95d8-fa78afa2d62e", Host: "192.0.2.1", Port: 443, SNI: "example.org", PublicKey: "fixture", ShortID: "aabb", Flow: cfg.Flow, Encryption: enc})
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(uri)
			if err != nil {
				t.Fatal(err)
			}
			q := strictURIQuery(t, u.RawQuery)
			if q["host"] != "aparat.com" || q["flow"] != c.Flow || q["encryption"] != enc {
				t.Fatal("connection identity changed")
			}
			if scope == "server" || scope == "disabled" {
				if q["fm"] != "" {
					t.Fatal("inactive client mask exported")
				}
				return
			}
			var legacy struct {
				TCP []struct {
					Type     string
					Settings struct {
						Packets  string
						Length   string
						Delay    string
						MaxSplit string
					}
				}
			}
			if err = json.Unmarshal([]byte(q["fm"]), &legacy); err != nil {
				t.Fatal(err)
			}
			if len(legacy.TCP) != 1 {
				t.Fatal("missing legacy mask")
			}
			s := legacy.TCP[0].Settings
			if s.Length != "48-80" || s.Delay != "0-1" || s.MaxSplit != "3-5" || s.Packets != "tlshello" {
				t.Fatalf("legacy core cannot use exported fragment: length=%q delay=%q", s.Length, s.Delay)
			}
			var modern struct {
				TCP []struct {
					Settings struct {
						Lengths []string
						Delays  []string
					}
				}
			}
			if err = json.Unmarshal([]byte(q["fm"]), &modern); err != nil {
				t.Fatal(err)
			}
			if len(modern.TCP[0].Settings.Lengths) != 5 || modern.TCP[0].Settings.Lengths[4] != "288-448" || modern.TCP[0].Settings.Delays[2] != "1-2" {
				t.Fatal("modern per-segment schedule changed")
			}
		})
	}
}
