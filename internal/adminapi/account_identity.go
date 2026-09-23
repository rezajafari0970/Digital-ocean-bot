package adminapi

import (
	"net/http"
)

func (s *Server) accountIdentity(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	id := r.PathValue("id")
	rt, err := s.Container.Runtime(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "account_not_ready"})
		return
	}
	a, err := rt.Provider.GetAccount(r.Context())
	if rt.Gateway != nil {
		rt.Gateway.CloseIdleConnections()
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "provider_identity_failed", "detail": err.Error()})
		return
	}
	if a.UUID == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "provider_identity_missing"})
		return
	}
	_, err = s.DB.ExecContext(r.Context(), `UPDATE accounts SET external_id=$2,email=NULLIF($3,''),updated_at=now() WHERE id=$1`, id, a.UUID, a.Email)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "identity_save_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "external_id": a.UUID, "email": a.Email})
}
