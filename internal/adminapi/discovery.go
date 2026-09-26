package adminapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
)

func (s *Server) accountDiscovery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Collapse accidental double taps/reloads: a successful snapshot younger
	// than 15 seconds is already fresh enough for an explicit UI refresh.
	var recentRaw []byte
	var recentAt time.Time
	var semanticState, observationError string
	_ = s.DB.QueryRowContext(r.Context(), `SELECT provider_state,COALESCE(provider_error_state,'') FROM accounts WHERE id=$1`, id).Scan(&semanticState, &observationError)
	if semanticState == app.ProviderStateActive && observationError == "" {
		if err := s.DB.QueryRowContext(r.Context(), `SELECT data,created_at FROM provider_snapshots WHERE account_id=$1 AND created_at > now()-interval '15 seconds' ORDER BY created_at DESC LIMIT 1`, id).Scan(&recentRaw, &recentAt); err == nil {
			var recent struct {
				Account struct {
					Email, UUID  string
					DropletLimit int `json:"droplet_limit"`
				} `json:"Account"`
				Droplets []json.RawMessage `json:"Droplets"`
			}
			if json.Unmarshal(recentRaw, &recent) == nil {
				writeJSON(w, 200, map[string]any{"refreshed": false, "cached": true, "email": recent.Account.Email, "external_id": recent.Account.UUID, "droplet_limit": recent.Account.DropletLimit, "provider_droplets": len(recent.Droplets), "snapshot_at": recentAt})
				return
			}
		}
	}
	runtime, err := s.Container.Runtime(r.Context(), id)
	if err != nil {
		providerState := app.ClassifyAccountProviderError(err, true)
		detail, _ := json.Marshal(map[string]any{"provider_state": providerState, "provider_error": err.Error(), "can_create": false})
		s.Container.RecordProviderObservation(r.Context(), id, providerState, err, string(detail))
		writeJSON(w, 409, map[string]string{"error": "account_not_ready", "state": providerState, "detail": err.Error()})
		return
	}
	result, err := runtime.Provider.Discover(r.Context())
	if runtime.Gateway != nil {
		runtime.Gateway.CloseIdleConnections()
	}
	if err != nil {
		var mode string
		_ = s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(mode,'') FROM network_profiles WHERE account_id=$1`, id).Scan(&mode)
		providerState := app.ClassifyAccountProviderError(err, mode == "proxy_required")
		detail, _ := json.Marshal(map[string]any{"provider_state": providerState, "provider_error": err.Error(), "can_create": false})
		s.Container.RecordProviderObservation(r.Context(), id, providerState, err, string(detail))
		writeJSON(w, 502, map[string]string{"error": "provider_discovery_failed", "state": providerState, "detail": err.Error()})
		return
	}
	raw, _ := json.Marshal(result)
	if _, err = s.DB.ExecContext(r.Context(), `INSERT INTO provider_snapshots(id,account_id,provider,version,data) VALUES(gen_random_uuid(),$1,'digitalocean',1,$2)`, id, raw); err != nil {
		writeJSON(w, 500, map[string]string{"error": "snapshot_store_failed"})
		return
	}
	canCreate := result.Account.Status == "active" && result.Account.DropletLimit > len(result.Droplets)
	detail, _ := json.Marshal(map[string]any{"provider_state": "ACTIVE", "provider_error": "", "can_create": canCreate, "droplet_limit": result.Account.DropletLimit, "provider_droplets": len(result.Droplets)})
	runtimeStatus := "READY"
	if !canCreate {
		runtimeStatus = "PROVIDER_BLOCKED"
	}
	_, _ = s.DB.ExecContext(r.Context(), `UPDATE accounts SET external_id=$2,email=NULLIF($3,''),provider_state='ACTIVE',provider_state_detail=$4,provider_state_at=now(),provider_error_state=NULL,provider_error_detail=NULL,provider_checked_at=now(),runtime_status=$5,runtime_status_detail=$4,runtime_status_at=now(),updated_at=now() WHERE id=$1`, id, result.Account.UUID, result.Account.Email, string(detail), runtimeStatus)
	writeJSON(w, 200, map[string]any{"refreshed": true, "email": result.Account.Email, "external_id": result.Account.UUID, "droplet_limit": result.Account.DropletLimit, "provider_droplets": len(result.Droplets)})
}
