package residentialsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"strings"
	"testing"
)

type batchProofCore struct {
	*fakeCore
	calls, reads int
	batch        []sanaei.SessionRequest
	fault        string
}

func (f *batchProofCore) Do(ctx context.Context, r sanaei.SessionRequest) (sanaei.SessionResponse, error) {
	if r.Path == "panel/api/xray/routeTest" {
		f.reads++
		if f.fault == "fallback_failed" {
			return sanaei.SessionResponse{}, context.DeadlineExceeded
		}
	}
	return f.fakeCore.Do(ctx, r)
}
func (f *batchProofCore) RouteProofBatch(ctx context.Context, rs []sanaei.SessionRequest) ([]sanaei.SessionResponse, error) {
	f.calls++
	f.batch = append([]sanaei.SessionRequest(nil), rs...)
	if f.fault == "fallback_failed" {
		return nil, fmt.Errorf("%w: ssh transport", sanaei.ErrRouteBatchUnavailable)
	}
	if f.fault == "unavailable" {
		return nil, sanaei.ErrRouteBatchUnavailable
	}
	if f.fault == "cancelled" {
		return nil, context.Canceled
	}
	out := []sanaei.SessionResponse{}
	for _, r := range rs {
		v, e := f.fakeCore.Do(ctx, r)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	if f.fault == "missing" {
		return out[:len(out)-1], nil
	}
	if f.fault == "mismatch" {
		out[len(out)-1].Body = json.RawMessage(`{"success":true,"obj":{"matched":true,"outboundTag":"wrong-plan"}}`)
	}
	return out, nil
}
func TestNativeRouteBatchPreservesCompleteMatrix(t *testing.T) {
	p := routePolicy{StrictAllowlist: true, Harden: true, AdsOnly: true, Residential: true, Direct: true, Configured: 1, Proxies: []rp{{Type: "socks5", Tag: "residential-ads-test"}}}
	clients, tags := fixtureClients(t, p)
	tags = append(tags, "inbound-trojan")
	desired, e := buildSettings(baseSettings(), clients, tags, p)
	if e != nil {
		t.Fatal(e)
	}
	total := len(tags) * 3 * 12
	for _, fault := range []string{"", "unavailable", "missing", "mismatch", "cancelled", "fallback_failed"} {
		t.Run(fault, func(t *testing.T) {
			f := &batchProofCore{fakeCore: &fakeCore{saved: desired, running: desired}, fault: fault}
			e := verifyRunning(context.Background(), f, desired, clients, tags, p)
			if fault == "" || fault == "unavailable" {
				if e != nil {
					t.Fatal(e)
				}
				if len(f.batch) != total {
					t.Fatal("incomplete full matrix", len(f.batch), total)
				}
				if fault == "" && f.reads != 1 || fault == "unavailable" && f.reads != total+1 {
					t.Fatal("missing internal DNS proof or fallback cases", f.reads)
				}
			} else if e == nil {
				t.Fatal("unverified batch accepted")
			}
			if fault == "mismatch" && !errors.Is(e, errRouteNotApplied) {
				t.Fatal(e)
			}
			if fault == "fallback_failed" && (!errors.Is(e, context.DeadlineExceeded) || !strings.Contains(e.Error(), "native route batch unavailable: ssh transport") || f.reads != 1) {
				t.Fatal("fallback lost its cause or continued after deadline", e, f.reads)
			}
			if f.saves != 0 || f.restarts != 0 {
				t.Fatal("read optimization mutated native core")
			}
		})
	}
}
