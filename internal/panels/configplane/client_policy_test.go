package configplane

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"reflect"
	"testing"
)

func TestGlobalPoliciesRequireExactOwnedIdentityAndLimits(t *testing.T) {
	cl := configdriver.Client{ID: "owned-id", Email: "owned-email", PolicySet: true, ExpiresAt: 1234, DeviceLimit: 7, QuotaBytes: 4096}
	c := configdriver.Credentials{Clients: []configdriver.Client{cl}}
	good := sanaei.GlobalClient{UUID: cl.ID, Email: cl.Email, ExpiryTime: cl.ExpiresAt, LimitHWID: cl.DeviceLimit, TotalGB: cl.QuotaBytes}
	unrelated := sanaei.GlobalClient{UUID: "other", Email: "other"}
	if e := verifyGlobalPolicies(c, []sanaei.GlobalClient{unrelated, unrelated, good}); e != nil {
		t.Fatal("unrelated record blocked owned policy", e)
	}
	if e := verifyGlobalPolicies(c, []sanaei.GlobalClient{good, good}); e == nil {
		t.Fatal("ambiguous owned identity accepted")
	}
	for _, bad := range []sanaei.GlobalClient{
		{UUID: "wrong", Email: cl.Email, ExpiryTime: cl.ExpiresAt, LimitHWID: cl.DeviceLimit, TotalGB: cl.QuotaBytes},
		{UUID: cl.ID, Email: cl.Email, ExpiryTime: cl.ExpiresAt, LimitHWID: 0, TotalGB: cl.QuotaBytes},
		{UUID: cl.ID, Email: cl.Email, ExpiryTime: 0, LimitHWID: cl.DeviceLimit, TotalGB: cl.QuotaBytes},
		{UUID: cl.ID, Email: cl.Email, ExpiryTime: cl.ExpiresAt, LimitHWID: cl.DeviceLimit, TotalGB: 0},
	} {
		if e := verifyGlobalPolicies(c, []sanaei.GlobalClient{bad}); e == nil {
			t.Fatal("mismatch accepted")
		}
	}
	if e := verifyGlobalPolicies(c, nil); e == nil {
		t.Fatal("missing identity accepted")
	}
}

func TestGlobalPolicyUpdatePreservesNativeFields(t *testing.T) {
	cl := configdriver.Client{ID: "owned", Email: "email", DeviceLimit: 5, ExpiresAt: 9000, QuotaBytes: 512}
	current := map[string]any{"uuid": "owned", "email": "email", "password": "secret", "enable": true, "flow": "xtls-rprx-vision", "limitIp": 4, "comment": "retain", "subId": "retain-sub", "allowedIPs": "192.0.2.1, 192.0.2.2", "limitHwid": 2, "totalGB": 10, "expiryTime": 100}
	got := globalPolicyPayload(current, cl)
	for _, key := range []string{"email", "password", "enable", "flow", "limitIp", "comment", "subId"} {
		if !reflect.DeepEqual(got[key], current[key]) {
			t.Fatalf("changed native field %s", key)
		}
	}
	if got["id"] != cl.ID || got["uuid"] != cl.ID || got["limitHwid"] != cl.DeviceLimit || got["totalGB"] != cl.QuotaBytes || got["expiryTime"] != cl.ExpiresAt {
		t.Fatal("requested policy absent")
	}
	if !reflect.DeepEqual(got["allowedIPs"], []string{"192.0.2.1", "192.0.2.2"}) {
		t.Fatal("allowed IPs lost")
	}
	if current["limitHwid"] != 2 {
		t.Fatal("input mutated")
	}
	current["allowedIPs"] = []any{"192.0.2.3"}
	if !reflect.DeepEqual(globalPolicyPayload(current, cl)["allowedIPs"], current["allowedIPs"]) {
		t.Fatal("array IPs lost")
	}
}
