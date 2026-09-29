package adminapi

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"net/http"
	"time"
)

func (s *Server) accountResources(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rows, err := s.DB.QueryContext(r.Context(), `SELECT r.id::text,r.provider_resource_id,r.type,r.state,r.managed,r.metadata,r.created_at,r.updated_at,a.provider FROM resources r JOIN accounts a ON a.id=r.account_id WHERE r.account_id=$1 ORDER BY updated_at DESC LIMIT 1000`, id)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var rid, pid, typ, state, provider string
		var managed bool
		var metadata []byte
		var created, updated any
		if rows.Scan(&rid, &pid, &typ, &state, &managed, &metadata, &created, &updated, &provider) != nil {
			continue
		}
		out = append(out, map[string]any{"id": rid, "provider_id": pid, "provider": provider, "provider_label": func() string {
			if provider == "vultr" {
				return "Vultr"
			}
			if provider == "digitalocean" {
				return "DigitalOcean"
			}
			return provider
		}(), "type": typ, "state": state, "managed": managed, "metadata": string(metadata), "created_at": created, "updated_at": updated})
	}
	writeJSON(w, 200, out)
}

func (s *Server) accountCapacity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	x, err := capacity.Read(r.Context(), s.DB, id, 2*time.Minute)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "account_capacity_unavailable"})
		return
	}
	writeJSON(w, 200, map[string]any{"server_limit": x.Limit, "provider_servers": x.InUse, "pending": x.Pending, "available": x.Available(), "droplet_limit": x.Limit, "active": x.InUse})
}
