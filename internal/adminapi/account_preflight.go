package adminapi

import (
	"encoding/json"
	"net/http"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
)

func (s *Server) accountPreflight(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id := r.PathValue("id")
	var enabled bool
	var regionsRaw, sizesRaw []byte
	var image, mode, proxyStatus string
	var interval, batch, maxConcurrent, lifetime, desired int
	err := s.DB.QueryRowContext(r.Context(), `SELECT a.enabled,a.preferred_regions,a.preferred_sizes,COALESCE(a.preferred_image,''),a.auto_interval_seconds,a.auto_batch_size,a.auto_max_concurrent,a.server_lifetime_seconds,a.desired_server_count,COALESCE(n.mode,''),COALESCE(p.status,'') FROM accounts a LEFT JOIN network_profiles n ON n.account_id=a.id LEFT JOIN proxies p ON p.id=n.proxy_id WHERE a.id=$1`, id).Scan(&enabled, &regionsRaw, &sizesRaw, &image, &interval, &batch, &maxConcurrent, &lifetime, &desired, &mode, &proxyStatus)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	var regions, sizes []string
	_ = json.Unmarshal(regionsRaw, &regions)
	_ = json.Unmarshal(sizesRaw, &sizes)
	checks := map[string]bool{
		"enabled":  enabled,
		"regions":  len(regions) > 0,
		"plan":     len(sizes) > 0,
		"image":    image != "",
		"lifetime": lifetime >= 1800,
		"schedule": interval >= 60 && batch >= 1 && maxConcurrent >= 1 && desired >= 1,
		"network":  mode == "direct" || (mode == "proxy_required" && (proxyStatus == "healthy")),
	}
	// Preflight validates identity with one lightweight provider request. Full
	// discovery belongs to explicit Refresh and must not be repeated here.
	rt, err := s.Container.Runtime(r.Context(), id)
	if err == nil {
		account, identityErr := rt.Provider.GetAccount(r.Context())
		if identityErr == nil {
			_, _ = s.DB.ExecContext(r.Context(), `UPDATE accounts SET external_id=$2,email=NULLIF($3,'') WHERE id=$1`, id, account.UUID, account.Email)
		}
		err = identityErr
		if rt.Gateway != nil {
			rt.Gateway.CloseIdleConnections()
		}
	}
	checks["provider_identity"] = err == nil
	providerError := ""
	if err != nil {
		providerError = err.Error()
	}
	ready := true
	for _, ok := range checks {
		if !ok {
			ready = false
		}
	}
	status := "PREFLIGHT_FAILED"
	providerState := app.ProviderStateActive
	if ready {
		status = "READY"
	} else if err != nil {
		providerState = app.ClassifyAccountProviderError(err, mode == "proxy_required")
		status = app.ProviderStateRuntimeStatus(providerState)
	}
	detail, _ := json.Marshal(map[string]any{"checks": checks, "provider_state": providerState, "provider_error": providerError, "can_create": ready})
	// Preflight's lightweight GetAccount success is not enough evidence to clear
	// a provider lock (other provider endpoints can still return 422). Only a
	// full Discover refresh can transition LOCKED -> ACTIVE.
	if err != nil {
		s.Container.RecordProviderObservation(r.Context(), id, providerState, err, string(detail))
	}
	code := 200
	if !ready {
		code = 409
	}
	writeJSON(w, code, map[string]any{"ready": ready, "status": status, "checks": checks, "provider_error": providerError})
}
