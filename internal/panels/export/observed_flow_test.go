package export

import (
	"encoding/base64"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"net/url"
	"strings"
	"testing"
)

func TestObservedXHTTPFlowMustMatchPin(t *testing.T) {
	c := transportprofile.Recommended()
	c.Path, _ = transportprofile.NewPath(c.PathBytes)
	_, enc, err := transportprofile.EncryptionPair(base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32))), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	x := VLESSReality{Transport: c, UUID: "fixture", Host: "192.0.2.1", Port: 443, SNI: "example.org", PublicKey: "fixture", ShortID: "abcd", Encryption: enc}
	for _, flow := range []string{"", transportprofile.VisionUDP443, "invalid"} {
		if _, err = transportprofile.ObservedClientFlow(c, flow); err == nil {
			t.Fatalf("accepted server drift %q", flow)
		}
	}
	x.Flow, err = transportprofile.ObservedClientFlow(c, transportprofile.Vision)
	if err != nil {
		t.Fatal(err)
	}
	link, err := VLESSRealityURI(x)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(link)
	if u.Query().Get("flow") != transportprofile.VisionUDP443 {
		t.Fatal("lost udp443")
	}
	for _, flow := range []string{"", transportprofile.Vision, "invalid"} {
		x.Flow = flow
		if _, err = VLESSRealityURI(x); err == nil {
			t.Fatalf("export silently repaired %q", flow)
		}
	}
	c = transportprofile.Preset()
	c.Path, _ = transportprofile.NewPath(c.PathBytes)
	if _, err = transportprofile.ObservedClientFlow(c, transportprofile.Vision); err == nil {
		t.Fatal("legacy empty pin drift accepted")
	}
	if flow, err := transportprofile.ObservedClientFlow(c, ""); err != nil || flow != "" {
		t.Fatal("legacy empty flow changed")
	}
}
