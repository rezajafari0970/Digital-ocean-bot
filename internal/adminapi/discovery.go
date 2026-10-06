package adminapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func (s *Server) accountDiscovery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Collapse accidental double taps/reloads: a successful snapshot younger
	// than 15 seconds is already fresh enough for an explicit UI refresh.
	var recentCanonical []byte
	var recentAt time.Time
	var semanticState, observationError string
	_ = s.DB.QueryRowContext(r.Context(), `SELECT provider_state,COALESCE(provider_error_state,'') FROM accounts WHERE id=$1`, id).Scan(&semanticState, &observationError)
	if semanticState == app.ProviderStateActive && observationError == "" {
		if err := s.DB.QueryRowContext(r.Context(), `SELECT canonical,created_at FROM provider_snapshots WHERE account_id=$1 AND canonical IS NOT NULL AND created_at > now()-interval '15 seconds' ORDER BY created_at DESC LIMIT 1`, id).Scan(&recentCanonical, &recentAt); err == nil {
			var recent providers.Observation
			if json.Unmarshal(recentCanonical, &recent) == nil {
				writeJSON(w, 200, map[string]any{"refreshed": false, "cached": true, "email": recent.Account.Email, "external_id": recent.Account.ID, "server_limit": recent.Capacity.ComputeLimit, "limit_known": recent.Capacity.LimitKnown, "provider_servers": recent.Capacity.ComputeInUse, "snapshot_at": recentAt})
				return
			}
		}
	}
	runtime, err := s.Container.Runtime(r.Context(), id)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "account_not_ready", "detail": err.Error()})
		return
	}
	sr, ok := runtime.Driver.(providers.ObservationReader)
	if !ok {
		writeJSON(w, 422, map[string]string{"error": "provider_observation_capability_missing"})
		return
	}
	obs, err := sr.Observe(r.Context())
	if runtime.Gateway != nil {
		runtime.Gateway.CloseIdleConnections()
	}
	if err != nil {
		var mode string
		_ = s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(mode,'') FROM network_profiles WHERE account_id=$1`, id).Scan(&mode)
		state := app.ClassifyAccountProviderError(err, mode == "proxy_required")
		s.Container.RecordProviderObservation(r.Context(), id, state, err, err.Error())
		writeJSON(w, 502, map[string]string{"error": "provider_discovery_failed", "state": state, "detail": err.Error()})
		return
	}
	if runtime.Config.Provider == "upcloud" {
		if err := s.Container.SyncUpCloudResources(r.Context(), id, obs.Inventory); err != nil {
			writeJSON(w, 500, map[string]string{"error": "inventory_store_failed"})
			return
		}
	}
	canonical, _ := json.Marshal(obs)
	if _, err = s.DB.ExecContext(r.Context(), `INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical) VALUES(gen_random_uuid(),$1,$2,2,$3,$4)`, id, runtime.Config.Provider, []byte(`{}`), canonical); err != nil {
		writeJSON(w, 500, map[string]string{"error": "snapshot_store_failed"})
		return
	}
	accountAllowed, statusErr := capacity.AccountStatusAllowsCreate(r.Context(), s.DB, id, obs.Account.Status)
	if statusErr != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	canCreate := accountAllowed && providers.CanCreateCapacity(obs.Capacity)
	detail, _ := json.Marshal(map[string]any{"provider_state": "ACTIVE", "provider_error": "", "can_create": canCreate, "server_limit": obs.Capacity.ComputeLimit, "limit_known": obs.Capacity.LimitKnown, "provider_servers": obs.Capacity.ComputeInUse})
	status := "READY"
	if !canCreate {
		status = "PROVIDER_BLOCKED"
	}
	_, _ = s.DB.ExecContext(r.Context(), `UPDATE accounts SET external_id=$2,email=NULLIF($3,''),provider_state='ACTIVE',provider_state_detail=$4,provider_state_at=now(),provider_error_state=NULL,provider_error_detail=NULL,provider_checked_at=now(),runtime_status=$5,runtime_status_detail=$4,runtime_status_at=now(),updated_at=now() WHERE id=$1`, id, obs.Account.ID, obs.Account.Email, string(detail), status)
	writeJSON(w, 200, map[string]any{"refreshed": true, "email": obs.Account.Email, "external_id": obs.Account.ID, "server_limit": obs.Capacity.ComputeLimit, "limit_known": obs.Capacity.LimitKnown, "provider_servers": obs.Capacity.ComputeInUse, "droplet_limit": obs.Capacity.ComputeLimit, "provider_droplets": obs.Capacity.ComputeInUse})
}
