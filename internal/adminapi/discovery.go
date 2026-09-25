package adminapi

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) accountDiscovery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Collapse accidental double taps/reloads: a successful snapshot younger
	// than 15 seconds is already fresh enough for an explicit UI refresh.
	var recentRaw []byte
	var recentAt time.Time
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
	runtime, err := s.Container.Runtime(r.Context(), id)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "account_not_ready", "detail": err.Error()})
		return
	}
	result, err := runtime.Provider.Discover(r.Context())
	if runtime.Gateway != nil {
		runtime.Gateway.CloseIdleConnections()
	}
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "provider_discovery_failed", "detail": err.Error()})
		return
	}
	raw, _ := json.Marshal(result)
	if _, err = s.DB.ExecContext(r.Context(), `INSERT INTO provider_snapshots(id,account_id,provider,version,data) VALUES(gen_random_uuid(),$1,'digitalocean',1,$2)`, id, raw); err != nil {
		writeJSON(w, 500, map[string]string{"error": "snapshot_store_failed"})
		return
	}
	_, _ = s.DB.ExecContext(r.Context(), `UPDATE accounts SET external_id=$2,email=NULLIF($3,''),updated_at=now() WHERE id=$1`, id, result.Account.UUID, result.Account.Email)
	writeJSON(w, 200, map[string]any{"refreshed": true, "email": result.Account.Email, "external_id": result.Account.UUID, "droplet_limit": result.Account.DropletLimit, "provider_droplets": len(result.Droplets)})
}
