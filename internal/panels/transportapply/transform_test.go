package transportapply

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/realitycontract"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/reality/credentials"
	"testing"
)

const testKey = "dob:global-reality:00443"

func fixture(t *testing.T) ([]byte, transportprofile.Config) {
	t.Helper()
	c := transportprofile.Preset()
	var e error
	c.Path, e = transportprofile.NewPath(c.PathBytes)
	if e != nil {
		t.Fatal(e)
	}
	k, e := (credentials.LocalX25519Generator{}).Generate(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	m := realitycontract.Managed{Transport: c, Remark: testKey, Host: "127.0.0.1", Port: 443, UUID: "11111111-1111-4111-8111-111111111111", Email: "kept", Target: "www.example.org", SNI: "www.example.org", PrivateKey: string(k.Private), PublicKey: k.Public, ShortID: "01234567"}
	p, e := m.Payload()
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(p)
	obj, _ := decode(raw)
	obj["id"] = json.Number("42")
	settings, _ := object(obj["settings"])
	settings["clients"] = []any{
		map[string]any{"id": m.UUID, "email": "kept", "flow": "", "totalGB": json.Number("9007199254740993"), "expiryTime": json.Number("1999999999999"), "limitHwid": json.Number("3"), "enable": true, "subId": "private", "tgId": json.Number("12345678901234567")},
		map[string]any{"id": "22222222-2222-4222-8222-222222222222", "email": "other", "flow": "", "totalGB": json.Number("42"), "expiryTime": json.Number("0"), "enable": false}}
	delete(settings, "flow")
	obj["settings"] = settings
	for _, key := range []string{"settings", "streamSettings", "sniffing"} {
		v, _ := json.Marshal(obj[key])
		obj[key] = string(v)
	}
	raw, _ = json.Marshal(obj)
	return raw, c
}
func TestTransformRoundTripPreservesClientState(t *testing.T) {
	before, old := fixture(t)
	if e := Verify(before, before, testKey, old); e != nil {
		t.Fatalf("legacy readback: %v", e)
	}
	defaults := transportprofile.Recommended()
	defaults.Host = "aparat.com"
	target, e := Target(old, defaults)
	if e != nil {
		t.Fatal(e)
	}
	if target.Path != old.Path {
		t.Fatal("unnecessary path churn")
	}
	after, p, e := Transform(before, testKey, target)
	if e != nil {
		t.Fatal(e)
	}
	if p.Settings["flow"] != transportprofile.Vision || p.Settings["decryption"] == "none" {
		t.Fatal("missing native Vision encryption")
	}
	if e = Verify(before, after, testKey, target); e != nil {
		t.Fatal(e)
	}
	// v3 retains an empty stored client flow and supplies settings.flow to Xray.
	obj, _ := decode(after)
	settings, _ := object(obj["settings"])
	settings["clients"].([]any)[0].(map[string]any)["flow"] = ""
	obj["settings"] = settings
	native, _ := json.Marshal(obj)
	if e = Verify(before, native, testKey, target); e != nil {
		t.Fatal(e)
	}
	if e = Verify(before, native, testKey, old); e == nil {
		t.Fatal("stale profile accepted")
	}
	rolled, _, e := Transform(native, testKey, old)
	if e != nil {
		t.Fatal(e)
	}
	if e = Verify(native, rolled, testKey, old); e != nil {
		t.Fatal("rollback", e)
	}
	original, _ := decode(before)
	a, _ := object(original["settings"])
	restored, _ := decode(rolled)
	z, _ := object(restored["settings"])
	ai, _ := clientInvariant(a)
	zi, _ := clientInvariant(z)
	if !jsonEqual(ai, zi) {
		t.Fatal("client state lost")
	}
	settings["clients"].([]any)[0].(map[string]any)["totalGB"] = json.Number("1")
	obj["settings"] = settings
	changed, _ := json.Marshal(obj)
	if Verify(before, changed, testKey, target) == nil {
		t.Fatal("changed quota accepted")
	}
	tcp := transportprofile.Config{Network: "tcp"}
	next, _, e := Transform(native, testKey, tcp)
	if e != nil {
		t.Fatal(e)
	}
	if e = Verify(native, next, testKey, tcp); e != nil {
		t.Fatal(e)
	}
}
func TestTransformRejectsUnsafeInputAndAllocatesPath(t *testing.T) {
	before, c := fixture(t)
	for _, alter := range []func(map[string]any){
		func(m map[string]any) { m["remark"] = "unmanaged" },
		func(m map[string]any) { m["enable"] = false },
		func(m map[string]any) {
			s, _ := object(m["settings"])
			s["fallbacks"] = []any{map[string]any{"dest": 80}}
			m["settings"] = s
		},
		func(m map[string]any) {
			s, _ := object(m["settings"])
			s["clients"] = append(s["clients"].([]any), s["clients"].([]any)[0])
			m["settings"] = s
		},
	} {
		m, _ := decode(before)
		alter(m)
		raw, _ := json.Marshal(m)
		if _, _, e := Transform(raw, testKey, c); e == nil {
			t.Fatal("unsafe input accepted")
		}
	}
	next := transportprofile.Recommended()
	next.PathBytes = 1024
	a, e := Target(c, next)
	if e != nil || len(a.Path) != 1024 || a.Path == c.Path {
		t.Fatal("new size", e)
	}
	b, e := Target(c, next)
	if e != nil || a.Path == b.Path {
		t.Fatal("path collision", e)
	}
}
