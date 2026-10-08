package sanaei

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels"
)

type AuthProvider interface {
	Authorize(context.Context, *http.Request) error
}

type Driver struct {
	HTTP *http.Client
	Auth AuthProvider
}

func (d Driver) Name() string { return "sanaei-3x-ui" }

func (d Driver) client() *http.Client {
	if d.HTTP != nil {
		return d.HTTP
	}
	return &http.Client{Timeout: 8 * time.Second, Transport: directPanelTransport}
}

func (d Driver) probe(ctx context.Context, in panels.Instance, method, path string) (int, error) {
	base := strings.TrimRight(in.BaseURL, "/") + "/"
	req, err := http.NewRequestWithContext(ctx, method, base+strings.TrimLeft(path, "/"), nil)
	if err != nil {
		return 0, err
	}
	if d.Auth != nil {
		if err = d.Auth.Authorize(ctx, req); err != nil {
			return 0, err
		}
	}
	resp, err := d.client().Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", panels.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func supported(code int) bool { return code >= 200 && code < 300 }
func exists(code int) bool    { return code != http.StatusNotFound && code != http.StatusMethodNotAllowed }

func (d Driver) Discover(ctx context.Context, in panels.Instance) (panels.Discovery, error) {
	ev := map[string]int{}
	probes := []struct{ k, m, p string }{
		{"inventory_slim", http.MethodGet, "panel/api/inbounds/list/slim"},
		{"inventory_full", http.MethodGet, "panel/api/inbounds/list"},
		{"server_status", http.MethodGet, "panel/api/server/status"},
		{"reality_scan", http.MethodPost, "panel/api/server/scanRealityTargets"},
	}
	for _, p := range probes {
		code, err := d.probe(ctx, in, p.m, p.p)
		if err != nil {
			return panels.Discovery{}, err
		}
		ev[p.k] = code
	}
	c := panels.Capabilities{
		InventorySlim: supported(ev["inventory_slim"]),
		InventoryFull: supported(ev["inventory_full"]),
		InboundRead:   supported(ev["inventory_full"]) || supported(ev["inventory_slim"]),
		ServerStatus:  supported(ev["server_status"]),
		RealityScan:   exists(ev["reality_scan"]),
	}
	return panels.Discovery{Driver: d.Name(), Version: in.Version, Capabilities: c, ObservedAt: time.Now().UTC(), Evidence: ev}, nil
}

func (d Driver) Health(ctx context.Context, in panels.Instance) error {
	code, err := d.probe(ctx, in, http.MethodGet, "panel/api/server/status")
	if err != nil {
		return err
	}
	if code == http.StatusUnauthorized || code == http.StatusForbidden {
		return panels.ErrUnauthorized
	}
	if !supported(code) {
		return fmt.Errorf("%w: status %d", panels.ErrUnavailable, code)
	}
	return nil
}

func DiscoverWithExecutor(ctx context.Context, exec SessionExecutor, version string) (panels.Discovery, error) {
	if exec == nil {
		return panels.Discovery{}, ErrSessionRequest
	}
	ev := map[string]int{}
	probes := []struct{ k, p string }{
		{"inventory_slim", "panel/api/inbounds/list/slim"},
		{"inventory_full", "panel/api/inbounds/list"},
		{"server_status", "panel/api/server/status"},
	}
	for _, p := range probes {
		r, err := exec.Do(ctx, SessionRequest{Method: http.MethodGet, Path: p.p})
		if err != nil {
			return panels.Discovery{}, err
		}
		ev[p.k] = r.StatusCode
	}
	c := panels.Capabilities{
		InventorySlim: supported(ev["inventory_slim"]),
		InventoryFull: supported(ev["inventory_full"]),
		InboundRead:   supported(ev["inventory_slim"]) || supported(ev["inventory_full"]),
		ServerStatus:  supported(ev["server_status"]),
	}
	return panels.Discovery{Driver: "sanaei-3x-ui", Version: version, Capabilities: c, ObservedAt: time.Now().UTC(), Evidence: ev}, nil
}
