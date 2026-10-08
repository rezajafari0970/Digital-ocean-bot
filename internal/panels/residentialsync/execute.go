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
	p = p.normalized()
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
	// Content-addressed tags cover the full domain list. Keep routine probes bounded;
	// additional domain-specific acceptance runs independently of this readiness window.
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
				{"adservice.google.com", "", "udp", "443", "quic", true},
				{"www.google.com", "", "tcp", "443", "tls", false},
				{"", "1.1.1.1", "udp", "53", "", false},
				{"browserleaks.com", "", "tcp", "443", "tls", true},
				{"", "1.1.1.1", "tcp", "53", "", false},
				{"", "1.1.1.1", "tcp", "443", "", false},
				{"www.gstatic.com", "", "tcp", "443", "tls", false},
				{"connectivitycheck.gstatic.com", "", "tcp", "80", "http", false},
				{"www.gstatic.com.example.org", "", "tcp", "443", "tls", false},
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
				if p.AdsOnly {
					explicitDirect := c.Effective == "DIRECT" && (!p.Harden && !p.AdsOnly || c.Class == "DIRECT")
					switch {
					case explicitDirect || (!p.StrictAllowlist && !p.Residential && p.Direct):
						base = directTag
					case !p.Residential || p.SniffingBlocked:
						base = blockedTag
					case p.StrictAllowlist && probe.port == "53":
						base = blockedTag
					case p.Harden && probe.port == "53":
						base = clientDNSTag
					case p.StrictAllowlist && !probe.ads && probe.domain != "www.gstatic.com" && probe.domain != "connectivitycheck.gstatic.com":
						base = blockedTag
					case !p.StrictAllowlist && !probe.ads:
						base = directTag
					case len(p.Proxies) > 0 && network == "udp" && ((!p.PoolEnabled && p.Proxies[0].Type != "socks5") || (p.PoolEnabled && !poolHasUDP(p))):
						base = blockedTag
					case len(p.Proxies) > 0:
						base = p.Proxies[0].Tag
					default:
						base = blockedTag
					}
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
				matches := json.Unmarshal(response.Body, &result) == nil && result.Obj.Matched && result.Obj.OutboundTag == expected
				if p.PoolEnabled && len(p.Proxies) > 0 && base == p.Proxies[0].Tag {
					matches = result.Obj.Matched && poolLaneMatches(desired, network, result.Obj.OutboundTag)
				}
				if !matches {
					return errRouteNotApplied
				}
			}
		}
	}

	if p.Harden {
		base := blockedTag
		if p.AdsOnly && (p.Residential || p.Direct) || !p.Residential && p.Direct {
			base = directTag
		} else if p.Residential && !p.SniffingBlocked && len(p.Proxies) > 0 {
			base = p.Proxies[0].Tag
		}
		port := "53"
		if p.StrictAllowlist {
			port = internalDNSPort(p)
		}
		form := url.Values{"port": {port}, "network": {"tcp"}, "inboundTag": {dnsTag}, "ip": {"1.1.1.1"}}
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
	if p.PoolEnabled {
		return verifyPoolOutcomes(ctx, exec, desired, p)
	}
	return nil
}

// The panel reports process=running before Xray binds its gRPC route API.
// A temporary RPC refusal is not evidence that the saved plan failed.
// Poll reads only; never repeat save/restart merely because readiness lags.
func verifyWhenReady(ctx context.Context, exec sanaei.SessionExecutor, desired map[string]any, clients []clientRoute, tags []string, p routePolicy) error {
	// This covers the whole route matrix, not only gRPC startup. Distant
	// panels need more than eight seconds for the sequential native requests.
	// The caller's 45-second operation deadline remains authoritative.
	readyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
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

// Probe the second routing stage as well: the first stage only returns loopback
// lanes. A blackhole outcome is a valid loaded plan when local probes are down.
func verifyPoolOutcomes(ctx context.Context, exec sanaei.SessionExecutor, desired map[string]any, p routePolicy) error {
	for _, network := range []string{"tcp", "udp"} {
		if len(p.Proxies) == 0 || network == "udp" && !poolHasUDP(p) {
			continue
		}
		for _, kind := range []string{"fast", "all"} {
			form := url.Values{"inboundTag": {poolPrefix + "in-" + network + "-" + kind}, "network": {network}, "port": {"443"}, "ip": {"1.1.1.1"}}
			if p.StrictAllowlist {
				form.Del("ip")
				form.Set("domain", "adservice.google.com")
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
			if json.Unmarshal(response.Body, &result) != nil || !result.Obj.Matched {
				return errRouteNotApplied
			}
			matches := result.Obj.OutboundTag == tagged(desired, blockedTag)
			for _, x := range p.Proxies {
				if network == "udp" && x.Type != "socks5" {
					continue
				}
				matches = matches || result.Obj.OutboundTag == tagged(desired, x.Tag)
			}
			if !matches {
				return errRouteNotApplied
			}
		}
	}
	return nil
}
