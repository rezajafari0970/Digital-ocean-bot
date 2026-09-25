package network

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
)

var ErrEgressChanged = errors.New("operation egress changed")

type EgressGuard struct {
	Client   *http.Client
	mu       sync.Mutex
	baseline string
}

func (g *EgressGuard) Observe(ctx context.Context) (string, error) {
	if g == nil || g.Client == nil {
		return "", ErrProxyConfigInvalid
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org", nil)
	resp, err := g.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(b))
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.baseline == "" {
		g.baseline = ip
		return ip, nil
	}
	if ip != g.baseline {
		return ip, ErrEgressChanged
	}
	return ip, nil
}
func (g *EgressGuard) Baseline() string { g.mu.Lock(); defer g.mu.Unlock(); return g.baseline }
