package residentialsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/url"
	"strings"
	"time"
)

var errRouteNotApplied = errors.New("running routing differs from plan")
var errRouteAPIStarting = errors.New("Xray route API not ready")

func envelope(resp sanaei.SessionResponse, err error) error {
	if err != nil {
		return err
	}
	var result struct {
		Success bool
		Msg     string
	}
	decoded := json.Unmarshal(resp.Body, &result) == nil
	if decoded && !result.Success && strings.Contains(result.Msg, "rpc error: code = Unavailable") {
		return errRouteAPIStarting
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !decoded || !result.Success {
		return fmt.Errorf("panel operation not confirmed (HTTP %d)", resp.StatusCode)
	}
	return nil
}
func applyAndVerify(ctx context.Context, exec sanaei.SessionExecutor, current, desired map[string]any, testURL string, clients []clientRoute, tags []string, p routePolicy, before ...func() error) error {
	if settingsHash(current) != settingsHash(desired) {
		for _, f := range before {
			if err := f(); err != nil {
				return err
			}
		}
		raw, _ := json.Marshal(desired)
		form := url.Values{"xraySetting": {string(raw)}, "outboundTestUrl": {testURL}}
		// Never infer success or failure from a lost response: always read the template.
		_, _ = exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/xray/update", ContentType: "application/x-www-form-urlencoded", Body: []byte(form.Encode()), TimeoutSeconds: 15})
		observed, _, err := readXraySetting(ctx, exec)
		if err != nil {
			return fmt.Errorf("template outcome unknown: %w", err)
		}
		if settingsHash(observed) != settingsHash(desired) {
			return errors.New("template update not observed")
		}
	}
	// Content-addressed outbound tags prove which complete plan the running core loaded.
	// They also resolve a lost restart response without restarting a second time.
	if err := verifyWhenReady(ctx, exec, desired, clients, tags, p); err == nil {
		return nil
	} else if !errors.Is(err, errRouteNotApplied) {
		return err
	}
	for _, f := range before {
		if err := f(); err != nil {
			return err
		}
	}
	_, _ = exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/server/restartXrayService", TimeoutSeconds: 15})
	if err := verifyWhenReady(ctx, exec, desired, clients, tags, p); err != nil {
		return fmt.Errorf("running routing not verified: %w", err)
	}
	return nil
}
func tagged(next map[string]any, base string) string {
	for _, v := range next["outbounds"].([]any) {
		m := v.(map[string]any)
		t, _ := m["tag"].(string)
		if t == base || strings.HasPrefix(t, base+"-") {
			return t
		}
	}
	return ""
}
func verifyRunning(ctx context.Context, exec sanaei.SessionExecutor, desired map[string]any, clients []clientRoute, tags []string, p routePolicy) error {
	resp, err := exec.Do(ctx, sanaei.SessionRequest{Method: "GET", Path: "panel/api/server/status", TimeoutSeconds: 5})
	if e := envelope(resp, err); e != nil {
		return e
	}
	var status struct {
		Obj struct{ Xray struct{ State string } }
	}
	if json.Unmarshal(resp.Body, &status) != nil || status.Obj.Xray.State != "running" {
		return errRouteNotApplied
	}
	// Every inbound and each represented class, plus an unassigned client, is checked.
	samples := []clientRoute{{Email: "dob-unassigned-route-probe", Effective: ""}}
	seen := map[string]bool{}
	for _, c := range clients {
		if !seen[c.Effective] {
			samples = append(samples, c)
			seen[c.Effective] = true
		}
	}
	if len(tags) == 0 {
		return nil
	}
	for _, tag := range tags {
		for _, c := range samples {
			for _, probe := range []struct {
				domain, ip, network, port, protocol string
				ads                                 bool
			}{
				{"adservice.google.com", "", "tcp", "443", "tls", true},
				{"pixel.facebook.com", "", "udp", "443", "quic", true},
				{"browserleaks.com", "", "tcp", "443", "tls", true},
				{"tls.browserleaks.com", "", "udp", "443", "quic", true},
				{"www.google.com", "", "tcp", "443", "tls", false},
				{"", "1.1.1.1", "udp", "53", "", false},
				{"", "1.1.1.1", "tcp", "443", "", false},
			} {
				network := probe.network
				base := blockedTag
				if c.Effective == "DIRECT" || (!p.Residential && p.Direct) || (c.Effective == "" && p.Configured == 0 && !p.Harden && (p.Residential || p.Direct)) {
					base = directTag
				} else if !p.SniffingBlocked && (c.Effective == "RESIDENTIAL" || c.Effective == "" && p.Residential) && len(p.Proxies) > 0 {
					if network == "tcp" || p.Proxies[0].Type == "socks5" {
						base = p.Proxies[0].Tag
					}
				}
				if p.AdsOnly && !probe.ads && (p.Residential || p.Direct) && (!p.Harden || probe.domain != "") && (!p.SniffingBlocked || c.Class == "DIRECT" || !p.Residential && p.Direct) {
					base = directTag
				}
				expected := tagged(desired, base)
				if expected == "" {
					return errors.New("routing proof target missing")
				}
				form := url.Values{"port": {probe.port}, "network": {network}, "inboundTag": {tag}, "email": {c.Email}, "protocol": {probe.protocol}}
				if probe.domain != "" {
					form.Set("domain", probe.domain)
				} else {
					form.Set("ip", probe.ip)
				}
				response, e := exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/xray/routeTest", ContentType: "application/x-www-form-urlencoded", Body: []byte(form.Encode()), TimeoutSeconds: 5})
				if e = envelope(response, e); e != nil {
					return e
				}
				var result struct {
					Obj struct {
						Matched     bool
						OutboundTag string
					}
				}
				if json.Unmarshal(response.Body, &result) != nil || !result.Obj.Matched || result.Obj.OutboundTag != expected {
					return errRouteNotApplied
				}
			}
		}
	}

	if p.Harden {
		base := blockedTag
		if !p.Residential && p.Direct {
			base = directTag
		} else if p.Residential && !p.SniffingBlocked && len(p.Proxies) > 0 && p.Proxies[0].Type == "socks5" {
			base = p.Proxies[0].Tag
		}
		form := url.Values{"port": {"53"}, "network": {"udp"}, "inboundTag": {dnsTag}, "ip": {"1.1.1.1"}}
		response, e := exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/xray/routeTest", ContentType: "application/x-www-form-urlencoded", Body: []byte(form.Encode()), TimeoutSeconds: 5})
		if e = envelope(response, e); e != nil {
			return e
		}
		var result struct {
			Obj struct {
				Matched     bool
				OutboundTag string
			}
		}
		if json.Unmarshal(response.Body, &result) != nil || !result.Obj.Matched || result.Obj.OutboundTag != tagged(desired, base) {
			return errRouteNotApplied
		}
	}
	return nil
}

// The panel reports process=running before Xray binds its gRPC route API.
// A temporary RPC refusal is not evidence that the saved plan failed.
// Poll reads only; never repeat save/restart merely because readiness lags.
func verifyWhenReady(ctx context.Context, exec sanaei.SessionExecutor, desired map[string]any, clients []clientRoute, tags []string, p routePolicy) error {
	readyCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	for {
		err := verifyRunning(readyCtx, exec, desired, clients, tags, p)
		if !errors.Is(err, errRouteAPIStarting) {
			return err
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-readyCtx.Done():
			timer.Stop()
			return err
		case <-timer.C:
		}
	}
}
