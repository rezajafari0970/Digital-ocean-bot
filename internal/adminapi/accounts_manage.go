package adminapi

import (
	"encoding/json"
	"net/http"
)

type accountUpdate struct {
	Name    string   `json:"name"`
	Token   string   `json:"token"`
	Regions []string `json:"regions"`
	Sizes   []string `json:"sizes"`
	Image   string   `json:"image"`
}

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id := r.PathValue("id")
	var x accountUpdate
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	regions, _ := json.Marshal(x.Regions)
	sizes, _ := json.Marshal(x.Sizes)
	res, err := s.DB.ExecContext(r.Context(), `UPDATE accounts SET name=$2,preferred_regions=$3,preferred_sizes=$4,preferred_image=NULLIF($5,''),updated_at=now() WHERE id=$1`, id, x.Name, regions, sizes, x.Image)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if x.Token != "" {
		if err := s.Container.Secrets.Put(r.Context(), id, "do-token", "digitalocean_token", []byte(x.Token)); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	w.WriteHeader(204)
}
func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id := r.PathValue("id")
	var active int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM deployments WHERE account_id=$1 AND state NOT IN ('READY','FAILED')`, id).Scan(&active)
	if active > 0 {
		writeJSON(w, 409, map[string]string{"error": "account_has_active_deployments"})
		return
	}
	res, err := s.DB.ExecContext(r.Context(), `DELETE FROM accounts WHERE id=$1`, id)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	w.WriteHeader(204)
}
