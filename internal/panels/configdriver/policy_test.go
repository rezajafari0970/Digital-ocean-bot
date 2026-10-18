package configdriver

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestClassPoliciesPreserveSiblingIdentityLifetimeAndRate(t *testing.T) {
	now := time.Unix(1800000000, 0)
	base, e := NewCredentials("profile", "panel", []string{"DIRECT"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	base.Clients = nil
	policies := map[string]ClassSettings{"DIRECT": {Count: 3, LifetimeSeconds: 60, DeviceLimit: 2, UsersPerSecond: 1, Publish: true}, "RESIDENTIAL": {Count: 2, LifetimeSeconds: 7200, DeviceLimit: 4, Publish: true}}
	first, e := ResizePolicies(base, "profile", "panel", []string{"DIRECT", "RESIDENTIAL"}, policies, now)
	if e != nil {
		t.Fatal(e)
	}
	if len(first.Clients) != 3 {
		t.Fatal("class creation limits were combined")
	}
	var residential []Client
	for _, c := range first.Clients {
		if c.Class == "RESIDENTIAL" {
			residential = append(residential, c)
			if c.ExpiresAt != now.Add(2*time.Hour).UnixMilli() || c.DeviceLimit != 4 {
				t.Fatal("residential settings lost")
			}
		} else if c.ExpiresAt != now.Add(time.Minute).UnixMilli() || c.DeviceLimit != 2 {
			t.Fatal("direct settings lost")
		}
	}
	immediate, e := ResizePolicies(first, "profile", "panel", []string{"DIRECT", "RESIDENTIAL"}, policies, now)
	if e != nil || !reflect.DeepEqual(first, immediate) {
		t.Fatal("retry changed identities or rate state", e)
	}
	second, e := ResizePolicies(first, "profile", "panel", []string{"DIRECT", "RESIDENTIAL"}, policies, now.Add(time.Second))
	if e != nil || len(second.Clients) != 4 {
		t.Fatal("direct rate did not advance independently", e)
	}
	direct := policies["DIRECT"]
	direct.Count = 1
	direct.DeviceLimit = 7
	policies["DIRECT"] = direct
	resized, e := ResizePolicies(second, "profile", "panel", []string{"DIRECT", "RESIDENTIAL"}, policies, now.Add(2*time.Second))
	if e != nil {
		t.Fatal(e)
	}
	if len(resized.Clients) != 3 || !reflect.DeepEqual(resized.Clients[1:], residential) {
		t.Fatal("direct edit changed residential users")
	}
	rotated, e := ResizePolicies(resized, "profile", "panel", []string{"DIRECT", "RESIDENTIAL"}, policies, now.Add(61*time.Second))
	if e != nil {
		t.Fatal(e)
	}
	if rotated.Clients[0].ID == resized.Clients[0].ID || !reflect.DeepEqual(rotated.Clients[1:], residential) {
		t.Fatal("expiry rotation was not isolated")
	}
}
func TestClassOutputFlowMaskAndCertificateOptions(t *testing.T) {
	now := time.Now()
	base, e := NewCredentials("profile", "panel", []string{"DIRECT"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	base.Clients = nil
	policies := map[string]ClassSettings{"DIRECT": {Count: 1, Publish: true, Flow: "xtls-rprx-vision", FinalMask: transportprofile.ActiveFinalMaskPreset, ExportFinalMask: true}, "RESIDENTIAL": {Count: 1, Publish: true}}
	creds, e := ResizePolicies(base, "profile", "panel", []string{"DIRECT", "RESIDENTIAL"}, policies, now)
	if e != nil {
		t.Fatal(e)
	}
	spec := Spec{Protocol: "vless", Network: "tcp", Security: "reality", SNI: "example.com", Port: 443}
	r := Builtins()
	for _, c := range creds.Clients {
		uri, e := r.URI(spec, "192.0.2.1", "🇩🇪 Germany", creds, c)
		if e != nil {
			t.Fatal(e)
		}
		u, _ := url.Parse(uri)
		if c.Class == "DIRECT" {
			if u.Query().Get("flow") == "" || u.Query().Get("finalmask") == "" {
				t.Fatal("enabled options missing")
			}
		} else if u.Query().Has("flow") || u.Query().Has("finalmask") {
			t.Fatal("direct output options leaked")
		}
		if strings.Contains(u.RawQuery, "+") {
			t.Fatal("strict mobile URI encoding violated")
		}
	}
	for _, bad := range []ClassSettings{{Count: 1, LifetimeSeconds: -1}, {Count: 1, DeviceLimit: -1}, {Count: 1, AllowInsecure: true}, {Count: 1, FinalMask: `{"tcp":[]}`, ExportFinalMask: true}} {
		if bad.Validate(spec) == nil {
			t.Fatal("invalid class policy accepted")
		}
	}
}

func TestExplicitEmptyFlowOverridesInheritedRealityFlow(t *testing.T) {
	c, e := NewCredentials("p", "s", []string{"DIRECT"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	policy := map[string]ClassSettings{"DIRECT": {Count: 1, Publish: true, Flow: "xtls-rprx-vision"}}
	same, e := ResizePolicies(c, "p", "s", []string{"DIRECT"}, policy, time.Now(), "xtls-rprx-vision")
	if e != nil || same.Clients[0].PolicySet {
		t.Fatal("unchanged inherited Flow rewrote credentials", e)
	}
	policy["DIRECT"] = ClassSettings{Count: 1, Publish: true}
	changed, e := ResizePolicies(c, "p", "s", []string{"DIRECT"}, policy, time.Now(), "xtls-rprx-vision")
	if e != nil || !changed.Clients[0].PolicySet || changed.Clients[0].Flow != "" || changed.Clients[0].ID != c.Clients[0].ID {
		t.Fatal("explicit empty Flow ignored", e)
	}
}
