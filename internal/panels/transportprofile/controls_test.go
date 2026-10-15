package transportprofile_test

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/realitycontract"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/postflight"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
)

func TestEditableEncryptedXHTTPServerAndClientAgree(t *testing.T) {
	c := transportprofile.Recommended()
	c.Host = "custom.example.org"
	c.Extra = json.RawMessage(`{"xPaddingBytes":"256-512","noGRPCHeader":false,"scStreamUpServerSecs":"15-25"}`)
	c.BrowserDialer = "WebView"
	var err error
	c.Path, err = transportprofile.NewPath(c.PathBytes)
	if err != nil {
		t.Fatal(err)
	}
	private := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	m := realitycontract.Managed{Transport: c, Remark: "listener-one", Port: 443, UUID: "11111111-1111-4111-8111-111111111111", Email: "fixture", Host: "192.0.2.2", Target: "www.example.org", SNI: "www.example.org", PrivateKey: private, PublicKey: "fixture-public", ShortID: "aabbccdd"}
	p, err := m.Payload()
	if err != nil {
		t.Fatal(err)
	}
	if p.Settings["flow"] != transportprofile.Vision {
		t.Fatal("server flow")
	}
	if _, ok := p.Settings["fallbacks"]; ok {
		t.Fatal("encryption with fallbacks")
	}
	in := postflight.Inbound{ID: 1, Remark: p.Remark, Port: p.Port, Protocol: p.Protocol, Enable: p.Enable, Settings: p.Settings, StreamSettings: p.StreamSettings, Sniffing: p.Sniffing}
	if err = postflight.ValidatePayload(in, 1, p); err != nil {
		t.Fatal("encrypted postflight", err)
	}
	dec := p.Settings["decryption"].(string)
	enc, err := transportprofile.ClientEncryption(dec)
	if err != nil || enc != p.Settings["encryption"] {
		t.Fatal("encryption pair")
	}
	if strings.HasSuffix(dec, private) {
		t.Fatal("REALITY private key was reused")
	}
	uri, err := m.URI()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("flow") != transportprofile.VisionUDP443 || q.Get("encryption") != enc || q.Get("host") != c.Host || q.Get("extra") != string(c.Extra) || q.Get("fm") == "" {
		t.Fatal("client/server contract diverged")
	}
	if strings.Contains(uri, private) || strings.Contains(uri, dec) || q.Has("bd") {
		t.Fatal("private key or invented dialer field exported")
	}
	observed, err := transportprofile.FromStream(p.StreamSettings)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := transportprofile.ApplyPinnedClientOptions(observed, c)
	if err != nil || pinned.Flow != c.Flow || pinned.BrowserDialer != "WebView" {
		t.Fatal("pin readback", err)
	}
	observed.Host = "changed.example.org"
	if _, err = transportprofile.ApplyPinnedClientOptions(observed, c); err == nil {
		t.Fatal("drift accepted")
	}
	dec2, enc2, err := transportprofile.EncryptionPair(private, m.Remark)
	if err != nil || dec2 != dec || enc2 != enc {
		t.Fatal("retry churn")
	}
	_, other, err := transportprofile.EncryptionPair(private, "listener-two")
	if err != nil || other == enc {
		t.Fatal("listener separation")
	}
	for _, scope := range []string{"client", "server", "both"} {
		m.Transport.FinalMaskScope = scope
		p, err = m.Payload()
		if err != nil {
			t.Fatal(err)
		}
		uri, err = m.URI()
		if err != nil {
			t.Fatal(err)
		}
		u, _ = url.Parse(uri)
		_, server := p.StreamSettings["finalmask"]
		if server != (scope != "client") || u.Query().Has("fm") != (scope != "server") {
			t.Fatal("mask scope", scope)
		}
	}
}

func TestTransportControlsRejectInvalidWireValues(t *testing.T) {
	for _, change := range []func(*transportprofile.Config){
		func(c *transportprofile.Config) { c.Flow = "udp443" },
		func(c *transportprofile.Config) { c.Host = "x.test\r\nInjected: yes" },
		func(c *transportprofile.Config) { c.Extra = json.RawMessage(`{"noGRPCHeader":"true"}`) },
		func(c *transportprofile.Config) { c.Extra = json.RawMessage(`{"serverMaxHeaderBytes":8192}`) },
		func(c *transportprofile.Config) { c.Extra = json.RawMessage(`{"headers":{"Host":"x.test"}}`) },
		func(c *transportprofile.Config) { c.Extra = json.RawMessage(`{"unknown":1}`) },
		func(c *transportprofile.Config) {
			c.FinalMask = json.RawMessage(`{"tcp":[{"type":"unknown","settings":{}}]}`)
		},
		func(c *transportprofile.Config) {
			c.FinalMask = json.RawMessage(`{"tcp":[{"type":"fragment","settings":{"packets":"tlshello","lengths":["0-1"],"delays":["0-1"]}}]}`)
		},
	} {
		c := transportprofile.Recommended()
		change(&c)
		if c.Validate(false) == nil {
			t.Fatal("invalid control accepted")
		}
	}
	c := transportprofile.Recommended()
	c.Mode = "packet-up"
	c.Host = "changed.example.org"
	c.BrowserDialer = "OkHttp"
	if err := c.Validate(false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.BrowserDialerStatus(), "not active") {
		t.Fatal("dialer incorrectly advertised as active")
	}
	c.FinalMask = json.RawMessage(transportprofile.FinalMaskPreset)
	mask, err := c.TCPMask()
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	json.Unmarshal(mask, &fields)
	if len(fields) != 1 || fields["tcp"] == nil {
		t.Fatal("inactive UDP/QUIC fields exported")
	}
}
