package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"net/http"
)

type profileWrite struct {
	AccountID string                   `json:"account_id"`
	Name      string                   `json:"name"`
	Config    workflow.ProfileSnapshot `json:"config"`
}

func (s *Server) createProfile(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanWrite() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x profileWrite
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.AccountID == "" || x.Name == "" || x.Config.Region == "" || x.Config.Size == "" || x.Config.Image == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	raw, _ := json.Marshal(x.Config)
	var id string
	err := s.DB.QueryRowContext(r.Context(), `INSERT INTO deployment_profiles(id,account_id,name,version,config) VALUES(gen_random_uuid(),$1,$2,1,$3) RETURNING id::text`, x.AccountID, x.Name, raw).Scan(&id)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}
