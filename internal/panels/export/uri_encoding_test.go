package export

import (
	"encoding/base64"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"net/url"
	"strings"
	"testing"
)

// v2rayNG Utils.decodeURIComponent preserves literal '+' (e1b73f2f).
// PathUnescape models that consumer contract; QueryUnescape hides this defect.
func strictURIQuery(t *testing.T, raw string) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, part := range strings.Split(raw, "&") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			t.Fatal("missing query value")
		}
		decoded, err := url.PathUnescape(value)
		if err != nil {
			t.Fatal(err)
		}
		result[key] = decoded
	}
	return result
}
func TestXHTTPURIComponentPreservesJSONTypesAndLiteralPlus(t *testing.T) {
	for _, encrypted := range []bool{false, true} {
		name := "legacy"
		if encrypted {
			name = "vision-finalmask"
		}
		t.Run(name, func(t *testing.T) {
			c := transportprofile.Preset()
			enc := "none"
			if encrypted {
				c = transportprofile.Recommended()
				_, enc, _ = transportprofile.EncryptionPair(base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32))), "encoding-fixture")
			}
			c.Path, _ = transportprofile.NewPath(4096)
			c.Extra = json.RawMessage(`{
 "noGRPCHeader": true,
 "scMaxEachPostBytes": 1000000,
 "scMinPostsIntervalMs": 30,
 "scMaxConcurrentPosts": 4,
 "scStreamUpServerSecs": "20-80",
 "xPaddingBytes": "100-1000",
 "headers": {"X-Probe": "space + literal & = %20 فارسی"}
}`)
			remark := "تهران + Paris & 50%"
			link, err := VLESSRealityURI(VLESSReality{Transport: c, Flow: c.Flow, Encryption: enc, UUID: "b530c072-21c6-405d-95d8-fa78afa2d62e", Host: "192.0.2.1", Port: 443, SNI: "example.org", PublicKey: "fixture", ShortID: "aabb", Remark: remark})
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(link)
			if err != nil {
				t.Fatal(err)
			}
			q := strictURIQuery(t, u.RawQuery)
			var extra struct {
				NoGRPCHeader bool              `json:"noGRPCHeader"`
				MaxBytes     int               `json:"scMaxEachPostBytes"`
				Headers      map[string]string `json:"headers"`
			}
			if err = json.Unmarshal([]byte(q["extra"]), &extra); err != nil {
				t.Fatal("v2rayNG-decoded Extra is invalid JSON:", err)
			}
			if !extra.NoGRPCHeader || extra.MaxBytes != 1000000 || extra.Headers["X-Probe"] != "space + literal & = %20 فارسی" {
				t.Fatal("JSON types or string values changed")
			}
			if q["extra"] != string(c.Extra) || q["path"] != c.Path || q["flow"] != c.Flow || q["encryption"] != enc || u.Fragment != remark {
				t.Fatal("URI consumer roundtrip changed values")
			}
			if strings.Contains(u.RawQuery, "+") || !strings.Contains(u.RawQuery, "%20") || !strings.Contains(u.RawQuery, "%2B") {
				t.Fatal("nonconforming URI component encoding")
			}
			if encrypted && !json.Valid([]byte(q["fm"])) {
				t.Fatal("FinalMask JSON corrupted")
			}
			for key, value := range q {
				if u.Query().Get(key) != value {
					t.Fatal("form decoder compatibility changed", key)
				}
			}
		})
	}
}
