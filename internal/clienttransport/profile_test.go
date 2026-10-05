package clienttransport

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testURI = "vless://00000000-0000-4000-8000-000000000001@203.0.113.1:443?encryption=none&flow=xtls-rprx-vision-udp443&security=reality&sni=example.com&fp=chrome&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&sid=0123&type=tcp#test%20config"

func TestOverlayPreservesIdentityAndRejectsExistingMask(t *testing.T) {
	for _, preset := range []string{"tls-record-v1", "tcp-balanced-v1", "layered-balanced-v1"} {
		got, e := Apply(testURI, preset)
		if e != nil {
			t.Fatal(e)
		}
		before, _ := url.Parse(testURI)
		after, _ := url.Parse(got)
		q := after.Query()
		var m FinalMask
		if json.Unmarshal([]byte(q.Get("fm")), &m) != nil || len(m.TCP) == 0 {
			t.Fatal("mask missing")
		}
		q.Del("fm")
		after.RawQuery = q.Encode()
		bq := before.Query()
		before.RawQuery = bq.Encode()
		if after.String() != before.String() {
			t.Fatal("identity changed")
		}
		if _, e = Apply(got, preset); e == nil {
			t.Fatal("silently replaced an existing mask")
		}
	}
	for _, p := range []string{"", "off"} {
		if got, e := Apply(testURI, p); e != nil || got != testURI {
			t.Fatal("baseline rewritten")
		}
	}
	for _, raw := range []string{"garbage", strings.Replace(testURI, "security=reality", "security=tls", 1), strings.Replace(testURI, ":443", ":99999", 1)} {
		if _, e := Apply(raw, "layered-balanced-v1"); e == nil {
			t.Fatal("accepted invalid URI")
		}
	}
	if _, e := Build("custom"); e == nil {
		t.Fatal("unbounded custom mask accepted")
	}
}
func TestNativeFinalMaskProfiles(t *testing.T) {
	binary := os.Getenv("XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("native core opt-in")
	}
	for _, p := range []string{"off", "tls-record-v1", "tcp-balanced-v1", "layered-balanced-v1"} {
		t.Run(p, func(t *testing.T) {
			uri, e := Apply(testURI, p)
			if e != nil {
				t.Fatal(e)
			}
			c, e := XrayJSON(uri)
			if e != nil {
				t.Fatal(e)
			}
			b, _ := json.Marshal(c)
			file := filepath.Join(t.TempDir(), "client.json")
			if e = os.WriteFile(file, b, 0600); e != nil {
				t.Fatal(e)
			}
			out, e := exec.Command(binary, "run", "-test", "-c", file).CombinedOutput()
			if e != nil {
				t.Fatalf("native schema: %v %s", e, out)
			}
		})
	}
}
