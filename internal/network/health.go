package network

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	ErrHealthHTTP    = errors.New("proxy health endpoint failed")
	ErrHealthPayload = errors.New("invalid proxy health payload")
)

type HealthResult struct {
	Status    ProxyStatus
	ExitIP    string
	Latency   time.Duration
	CheckedAt time.Time
	Error     string
}

type ipPayload struct {
	IP string `json:"ip"`
}

func CheckProxy(ctx context.Context, g *Gateway, endpoint, expectedExitIP, serverPublicIP string) HealthResult {
	started := time.Now()
	result := HealthResult{Status: StatusDown, CheckedAt: started}
	if err := ctx.Err(); err != nil {
		result.Error = activeProbeDiagnostic(err)
		return result
	}
	if g == nil || g.Client == nil {
		result.Error = activeProbeDiagnostic(ErrProxyConfigInvalid)
		return result
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		result.Error = activeProbeDiagnostic(err)
		return result
	}
	resp, err := g.Client.Do(req)
	result.Latency = time.Since(started)
	if err != nil {
		result.Error = activeProbeDiagnostic(err)
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = activeProbeDiagnostic(ErrHealthHTTP)
		return result
	}
	var payload ipPayload
	// Read through EOF within a small bound: a valid prefix is not proof that
	// a second JSON value, trailing garbage or an oversized response is valid.
	const maxHealthResponseBytes = 16 << 10
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxHealthResponseBytes+1))
	if readErr != nil || len(body) > maxHealthResponseBytes || json.Unmarshal(body, &payload) != nil || strings.TrimSpace(payload.IP) == "" {
		result.Error = activeProbeDiagnostic(ErrHealthPayload)
		return result
	}
	result.ExitIP = strings.TrimSpace(payload.IP)
	if err := ValidateLeakObservation(LeakObservation{ObservedIP: result.ExitIP, ExpectedExitIP: expectedExitIP, ServerPublicIP: serverPublicIP, Headers: resp.Header}); err != nil {
		result.Error = activeProbeDiagnostic(err)
		return result
	}
	result.Status = StatusHealthy
	return result
}
