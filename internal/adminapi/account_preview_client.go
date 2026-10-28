package adminapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) previewClient(w http.ResponseWriter, r *http.Request, proxyID, country string, probe bool) (*http.Client, func(), string, bool) {
	c := s.Container
	c.DB = s.DB
	client, closeFn, previewExitIP, err := c.OnboardingClient(r.Context(), proxyID, country)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "preview_route_unavailable", "detail": err.Error()})
		return nil, nil, "", false
	}
	if probe && previewExitIP == "" && proxyID != "" {
		probeCtx, probeCancel := context.WithTimeout(r.Context(), 6*time.Second)
		defer probeCancel()
		probeReq, _ := http.NewRequestWithContext(probeCtx, http.MethodGet, "https://api.ipify.org", nil)
		if response, e := client.Do(probeReq); e == nil && response != nil {
			b, _ := io.ReadAll(io.LimitReader(response.Body, 128))
			response.Body.Close()
			if response.StatusCode/100 == 2 {
				previewExitIP = strings.TrimSpace(string(b))
			}
		}
	}
	return client, closeFn, previewExitIP, true
}
