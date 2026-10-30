package adminapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) previewClient(w http.ResponseWriter, r *http.Request, proxyID, country string, probe bool) (*http.Client, func(), string, bool) {
	return s.previewPriorities(w, r, normalizeProxyPool(proxyID, nil), []string{country}, probe)
}
func (s *Server) previewPriorities(w http.ResponseWriter, r *http.Request, ids, countries []string, probe bool) (*http.Client, func(), string, bool) {
	c := s.Container
	c.DB = s.DB
	route, err := c.OnboardingPriorities(r.Context(), ids, countries, false)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "preview_route_unavailable", "detail": err.Error()})
		return nil, nil, "", false
	}
	client, closeFn, previewExitIP := route.Client, route.Close, route.IP
	if probe && previewExitIP == "" && len(ids) > 0 {
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
