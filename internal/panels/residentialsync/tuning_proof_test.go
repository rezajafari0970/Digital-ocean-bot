package residentialsync

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"net/url"
	"strings"
	"testing"
)

type tuningProofCore struct{ fakeCore }

func (f *tuningProofCore) Do(ctx context.Context, r sanaei.SessionRequest) (sanaei.SessionResponse, error) {
	if r.Path == "panel/api/xray/routeTest" {
		v, _ := url.ParseQuery(string(r.Body))
		if strings.HasPrefix(v.Get("inboundTag"), poolPrefix) {
			b, _ := json.Marshal(map[string]any{"success": true, "obj": map[string]any{"matched": true, "outboundTag": tagged(f.running, "residential-ads-fixture")}})
			return sanaei.SessionResponse{StatusCode: 200, Body: b}, nil
		}
	}
	return f.fakeCore.Do(ctx, r)
}
func TestTuningEmptyClientTagsStillProveCandidateAndRestoration(t *testing.T) {
	p := routePolicy{PoolEnabled: true, Harden: true, AdsOnly: true, StrictAllowlist: true, Residential: true, Configured: 1, StableFingerprint: true,
		Proxies: []rp{{ID: "11111111-1111-4111-8111-111111111111", Type: "socks5", Host: "127.0.0.1", Port: 1080, Tag: "residential-ads-fixture"}}}
	before := residentialperf.Balanced()
	before.FastCount = 13
	before.FastOpen = true
	after := before
	after.FastCount = 3
	after.FastShare = 90
	base := map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"rules": []any{}}}
	p.Performance = &before
	parent, e := buildSettings(base, nil, nil, p)
	if e != nil {
		t.Fatal(e)
	}
	p.Performance = &after
	candidate, e := buildSettings(parent, nil, nil, p)
	if e != nil {
		t.Fatal(e)
	}
	f := &tuningProofCore{fakeCore{saved: parent, running: parent, loseSave: true, loseRestart: true}}
	if e = verifyRunning(context.Background(), f, candidate, nil, nil, p); e == nil {
		t.Fatal("old running plan acknowledged merely because tags are empty")
	}
	if e = applyAndVerify(context.Background(), f, parent, candidate, "", nil, nil, p); e != nil {
		t.Fatal(e)
	}
	if f.saves != 1 || f.restarts != 1 {
		t.Fatal("candidate not actually loaded", f.saves, f.restarts)
	}
	if e = applyAndVerify(context.Background(), f, candidate, candidate, "", nil, nil, p); e != nil {
		t.Fatal(e)
	}
	if f.restarts != 1 {
		t.Fatal("lost response caused repeated restart")
	}
	p.Performance = &before
	if e = applyAndVerify(context.Background(), f, candidate, parent, "", nil, nil, p); e != nil {
		t.Fatal(e)
	}
	if f.saves != 2 || f.restarts != 2 {
		t.Fatal("restored template without loaded-plan proof")
	}
}
func TestTuningEmptyClientTagsWithoutInternalProofFailClosed(t *testing.T) {
	f := &fakeCore{}
	if e := verifyRunning(context.Background(), f, map[string]any{}, nil, nil, routePolicy{}); e == nil {
		t.Fatal("process state used as native configuration proof")
	}
}
