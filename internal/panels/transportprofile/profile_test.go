package transportprofile_test

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/realitycontract"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/postflight"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"net/url"
	"strings"
	"testing"
)

func TestPathLengthUniquenessAndBoundaries(t *testing.T) {
	all := map[string]bool{}
	for _, n := range []int{256, 687, 1024, 4096} {
		for i := 0; i < 40; i++ {
			p, e := transportprofile.NewPath(n)
			if e != nil || len(p) != n || all[p] {
				t.Fatal(n, len(p), e)
			}
			all[p] = true
			if transportprofile.ValidatePath(p, n) != nil {
				t.Fatal("invalid path")
			}
			u, e := url.Parse("https://apart.com" + p)
			if e != nil || u.EscapedPath() != p {
				t.Fatal("wire encoding changed")
			}
		}
	}
	for _, n := range []int{-1, 0, 255, 4097, 1 << 20} {
		if _, e := transportprofile.NewPath(n); e == nil {
			t.Fatal(n)
		}
	}
}
func TestInvalidOptionsAndLegacyRequestedPreset(t *testing.T) {
	base := transportprofile.Preset()
	for _, mutate := range []func(*transportprofile.Config){
		func(c *transportprofile.Config) { c.Host = "apart.com\nx:test" },
		func(c *transportprofile.Config) {
			c.FinalMaskEnabled = true
			c.FinalMask = append(append([]byte("{\"finalmask\":"), c.FinalMask...), '}')
		},
	} {
		c := base
		mutate(&c)
		if c.Validate(false) == nil {
			t.Fatal("unsupported option accepted")
		}
	}
	var mask map[string]json.RawMessage
	if json.Unmarshal(base.FinalMask, &mask) != nil || mask["tcp"] == nil || mask["udp"] == nil || mask["quicParams"] == nil || mask["finalmask"] != nil {
		t.Fatal("FinalMask wrapper")
	}
	if !strings.Contains(string(base.FinalMask), `"brutalUp":0`) {
		t.Fatal("requested types changed")
	}
}
func TestServerURIAndReadbackAgree(t *testing.T) {
	c := transportprofile.Preset()
	var e error
	c.Path, e = transportprofile.NewPath(c.PathBytes)
	if e != nil {
		t.Fatal(e)
	}
	m := realitycontract.Managed{Transport: c, Remark: "managed", Port: 443, UUID: "11111111-1111-4111-8111-111111111111", Email: "test", Host: "192.0.2.1", Target: "aws.amazon.com", SNI: "aws.amazon.com", PrivateKey: "fixture-private", PublicKey: "fixture-public", ShortID: "aabbccdd"}
	p, e := m.Payload()
	if e != nil {
		t.Fatal(e)
	}
	uri, e := m.URI()
	if e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(uri)
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	if q.Get("flow") != "" || q.Has("flow") || q.Get("fm") != "" || q.Get("type") != "xhttp" || q.Get("host") != "apart.com" || q.Get("sni") != "aws.amazon.com" || q.Get("path") != c.Path || q.Get("mode") != "stream-up" {
		t.Fatal("URI mismatch")
	}
	if q.Get("extra") != string(c.Extra) {
		t.Fatal("Extra mismatch")
	}
	in := postflight.Inbound{ID: 1, Remark: p.Remark, Port: p.Port, Protocol: p.Protocol, Enable: p.Enable, Settings: p.Settings, StreamSettings: p.StreamSettings, Sniffing: p.Sniffing}
	if e = postflight.ValidatePayload(in, 1, p); e != nil {
		t.Fatal(e)
	}
	for _, field := range []string{"host", "path", "mode", "extra"} {
		b, _ := json.Marshal(p.StreamSettings)
		var st map[string]any
		json.Unmarshal(b, &st)
		st["xhttpSettings"].(map[string]any)[field] = "wrong"
		in.StreamSettings = st
		if postflight.ValidatePayload(in, 1, p) == nil {
			t.Fatal("tampered", field)
		}
	}
	m.Flow = "xtls-rprx-vision"
	if _, e = m.Payload(); e == nil {
		t.Fatal("Vision payload")
	}
	if _, e = m.URI(); e == nil {
		t.Fatal("Vision URI")
	}
}
