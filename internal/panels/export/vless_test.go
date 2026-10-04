package export

import (
	"net/url"
	"strings"
	"testing"
)

func TestRealityURIAlwaysCarriesVisionFlow(t *testing.T) {
	s, e := VLESSRealityURI(VLESSReality{UUID: "eb44dbca-6064-4085-8145-ca022e5b2f65", Host: "167.172.28.228", Port: 18443, SNI: "www.cloudflare.com", PublicKey: "public", ShortID: "04a54a16", Remark: "test"})
	if e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(s)
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	if q.Get("flow") != "xtls-rprx-vision-udp443" {
		t.Fatalf("flow=%q", q.Get("flow"))
	}
	for _, k := range []string{"security", "sni", "fp", "pbk", "sid", "type"} {
		if strings.TrimSpace(q.Get(k)) == "" {
			t.Fatalf("missing %s", k)
		}
	}
}
