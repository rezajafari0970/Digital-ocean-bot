package healthverify

import "testing"

func TestParseVLESSReality(t *testing.T) {
	c, err := ParseVLESSReality("vless://11111111-1111-1111-1111-111111111111@203.0.113.7:443?security=reality&sni=example.com&pbk=pub&sid=abcd&fp=chrome&flow=xtls-rprx-vision&type=tcp#x")
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "203.0.113.7" || c.Port != 443 || c.SNI != "example.com" || c.ShortID != "abcd" {
		t.Fatalf("%+v", c)
	}
}
