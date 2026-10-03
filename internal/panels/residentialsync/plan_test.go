package residentialsync

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/url"
	"strings"
	"testing"
)

func baseSettings() map[string]any {
	return map[string]any{"api": map[string]any{"tag": "api"}, "unknown": json.Number("9007199254740993"), "outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}, map[string]any{"tag": "blocked", "protocol": "blackhole"}}, "routing": map[string]any{"rules": []any{map[string]any{"type": "field", "inboundTag": []any{"api"}, "outboundTag": "api"}}}}
}
func fixtureClients(t *testing.T, p routePolicy) ([]clientRoute, []string) {
	t.Helper()
	raw := json.RawMessage(`{"id":1,"tag":"actual-inbound-tag","protocol":"vless","settings":{"clients":[{"id":"a","email":"a@test"},{"id":"b","email":"b@test"},{"id":"c","email":"c@test"},{"id":"d","email":"d@test"}]}}`)
	cs, tags, err := planClients([]json.RawMessage{raw}, nil, p)
	if err != nil {
		t.Fatal(err)
	}
	return cs, tags
}
func TestRoutingClassesAndClosedFailure(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		p                    routePolicy
		direct, res, blocked int
	}{
		{"both", routePolicy{Residential: true, Direct: true, Configured: 1, Proxies: []rp{{Type: "socks5", Tag: "residential-ads-test"}}}, 2, 2, 0},
		{"down", routePolicy{Residential: true, Direct: true, Configured: 1}, 2, 0, 2},
		{"absent", routePolicy{Residential: true}, 4, 0, 0},
		{"direct", routePolicy{Direct: true, Configured: 1}, 4, 0, 0},
		{"disabledclasses", routePolicy{Configured: 1}, 0, 0, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs, tags := fixtureClients(t, tc.p)
			counts := map[string]int{}
			for _, c := range cs {
				counts[c.Effective]++
			}
			if counts["DIRECT"] != tc.direct || counts["RESIDENTIAL"] != tc.res || counts["BLOCKED"] != tc.blocked {
				t.Fatal(counts)
			}
			desired, err := buildSettings(baseSettings(), cs, tags, tc.p)
			if err != nil {
				t.Fatal(err)
			}
			again, err := buildSettings(desired, cs, tags, tc.p)
			if err != nil || settingsHash(desired) != settingsHash(again) {
				t.Fatal("plan unstable", err)
			}
			if desired["unknown"] != json.Number("9007199254740993") {
				t.Fatal("unknown value lost")
			}
		})
	}
}
func TestConflictingClientIdentityRejected(t *testing.T) {
	for _, raw := range []string{
		`{"id":1,"tag":"tag","protocol":"vless","settings":{"clients":[{"id":"a","email":"x"},{"id":"b","email":"X"}]}}`,
		`{"id":1,"tag":"tag","protocol":"vless","settings":{"clients":[{"id":"a","email":"regexp:.*"}]}}`,
	} {
		if _, _, err := planClients([]json.RawMessage{json.RawMessage(raw)}, nil, routePolicy{Direct: true}); err == nil {
			t.Fatal("unsafe inventory accepted")
		}
	}
}

type fakeCore struct {
	saved, running                               map[string]any
	saves, restarts                              int
	loseSave, loseRestart, dropSave, readFailure bool
}

func (f *fakeCore) Do(ctx context.Context, r sanaei.SessionRequest) (sanaei.SessionResponse, error) {
	ok := func(obj any) (sanaei.SessionResponse, error) {
		raw, _ := json.Marshal(map[string]any{"success": true, "obj": obj})
		return sanaei.SessionResponse{StatusCode: 200, Body: raw}, nil
	}
	switch r.Path {
	case "panel/api/xray/":
		if f.readFailure {
			return sanaei.SessionResponse{}, errors.New("read unavailable")
		}
		return ok(map[string]any{"xraySetting": f.saved})
	case "panel/api/xray/update":
		f.saves++
		v, _ := url.ParseQuery(string(r.Body))
		if !f.dropSave {
			f.saved, _ = decodeObject([]byte(v.Get("xraySetting")))
		}
		if f.loseSave {
			return sanaei.SessionResponse{}, errors.New("response lost")
		}
		return ok(nil)
	case "panel/api/server/status":
		return ok(map[string]any{"xray": map[string]any{"state": "running"}})
	case "panel/api/server/restartXrayService":
		f.restarts++
		f.running = f.saved
		if f.loseRestart {
			return sanaei.SessionResponse{}, errors.New("response lost")
		}
		return ok(nil)
	case "panel/api/xray/routeTest":
		v, _ := url.ParseQuery(string(r.Body))
		tag := ""
		raw, _ := json.Marshal(f.running)
		var setting map[string]any
		json.Unmarshal(raw, &setting)
		for _, value := range setting["routing"].(map[string]any)["rules"].([]any) {
			rule := value.(map[string]any)
			if ins, ok := rule["inboundTag"].([]any); ok {
				found := false
				for _, x := range ins {
					if x == v.Get("inboundTag") {
						found = true
					}
				}
				if !found {
					continue
				}
			}
			if users, ok := rule["user"].([]any); ok {
				found := false
				for _, x := range users {
					if x == v.Get("email") {
						found = true
					}
				}
				if !found {
					continue
				}
			}
			if n, ok := rule["network"].(string); ok && !strings.Contains(n, v.Get("network")) {
				continue
			}
			tag, _ = rule["outboundTag"].(string)
			break
		}
		return ok(map[string]any{"matched": tag != "", "outboundTag": tag})
	}
	return sanaei.SessionResponse{}, errors.New("unexpected request")
}
func TestLostSaveAndRestartResponsesReconcileWithoutDuplicate(t *testing.T) {
	p := routePolicy{Residential: true, Direct: true, Configured: 1, Proxies: []rp{{Type: "http", Host: "example.test", Port: 8080, Password: "test", Tag: "residential-ads-test"}}}
	cs, tags := fixtureClients(t, p)
	base := baseSettings()
	desired, e := buildSettings(base, cs, tags, p)
	if e != nil {
		t.Fatal(e)
	}
	f := &fakeCore{saved: base, running: base, loseSave: true, loseRestart: true}
	if e = applyAndVerify(context.Background(), f, base, desired, "", cs, tags, p); e != nil {
		t.Fatal(e)
	}
	if f.saves != 1 || f.restarts != 1 {
		t.Fatal(f.saves, f.restarts)
	}
	if e = applyAndVerify(context.Background(), f, desired, desired, "", cs, tags, p); e != nil {
		t.Fatal(e)
	}
	if f.saves != 1 || f.restarts != 1 {
		t.Fatal("duplicate after recovery")
	}
	// A credential edit changes the runtime proof even when the route class is unchanged.
	p.Proxies[0].Password = "changed"
	next, e := buildSettings(desired, cs, tags, p)
	if e != nil {
		t.Fatal(e)
	}
	if tagged(next, p.Proxies[0].Tag) == tagged(desired, p.Proxies[0].Tag) {
		t.Fatal("credential update lacks runtime proof")
	}
}
func TestUnobservedTemplateNeverRestartsOrBlindRetries(t *testing.T) {
	p := routePolicy{Direct: true}
	cs, tags := fixtureClients(t, p)
	base := baseSettings()
	desired, _ := buildSettings(base, cs, tags, p)
	for _, readFail := range []bool{false, true} {
		f := &fakeCore{saved: base, running: base, dropSave: true, readFailure: readFail}
		if err := applyAndVerify(context.Background(), f, base, desired, "", cs, tags, p); err == nil {
			t.Fatal("unverified success")
		}
		if f.saves != 1 || f.restarts != 0 {
			t.Fatal(f.saves, f.restarts)
		}
	}
}
