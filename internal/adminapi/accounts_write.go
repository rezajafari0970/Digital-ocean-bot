package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"net/http"
)

type accountWrite struct {
	Name    string `json:"name"`
	Token   string `json:"token"`
	Enabled *bool  `json:"enabled"`
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x accountWrite
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.Name == "" || x.Token == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	var id string
	err := s.DB.QueryRowContext(r.Context(), `INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'digitalocean',$1,'do-token') RETURNING id::text`, x.Name).Scan(&id)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	if err := s.Container.Secrets.Put(r.Context(), id, "do-token", "digitalocean_token", []byte(x.Token)); err != nil {
		_, _ = s.DB.ExecContext(r.Context(), `DELETE FROM accounts WHERE id=$1`, id)
		writeJSON(w, 500, errorBody())
		return
	}
	_, _ = s.DB.ExecContext(r.Context(), `INSERT INTO network_profiles(id,account_id,mode) VALUES(gen_random_uuid(),$1,'direct')`, id)
	writeJSON(w, 201, map[string]string{"id": id})
}
func requireAdmin(p auth.Principal) bool { return p.CanAdmin() }
