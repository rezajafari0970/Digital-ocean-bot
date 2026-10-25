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
			rows.Close()
			writeJSON(w, http.StatusInternalServerError, errorBody())
			return
		}
		out = append(out, map[string]any{"id": rid, "provider_id": pid, "provider": provider, "provider_label": func() string {
			if provider == "aws" {
				return "Amazon AWS EC2"
			}
			if provider == "ovh" {
				return "OVH Public Cloud"
			}
			if provider == "vultr" {
				return "Vultr"
			}
			if provider == "linode" {
				return "Linode"
			}
			if provider == "upcloud" {
				return "UpCloud"
			}
			if provider == "digitalocean" {
				return "DigitalOcean"
			}
			return provider
		}(), "type": typ, "state": state, "managed": managed, "metadata": string(metadata), "created_at": created, "updated_at": updated})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeJSON(w, http.StatusInternalServerError, errorBody())
		return
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
	var provider string
	if err := s.DB.QueryRowContext(r.Context(), "SELECT provider FROM accounts WHERE id=$1", id).Scan(&provider); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if (provider == "linode" || provider == "aws" || provider == "ovh") && !x.LimitKnown {
		writeJSON(w, 200, map[string]any{"server_limit": nil, "limit_known": false, "provider_servers": x.InUse, "pending": x.Pending, "available": nil, "droplet_limit": nil, "active": x.InUse})
		return
	}
	writeJSON(w, 200, map[string]any{"server_limit": x.Limit, "provider_servers": x.InUse, "pending": x.Pending, "available": x.Available(), "droplet_limit": x.Limit, "active": x.InUse})
}
