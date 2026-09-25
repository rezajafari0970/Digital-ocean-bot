package adminapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

type loginStateWrite struct {
	Status    string `json:"status"`
	Challenge string `json:"challenge"`
	Detail    string `json:"detail"`
}

func (s *Server) setLoginState(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var x loginStateWrite
	if json.NewDecoder(r.Body).Decode(&x) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	x.Status = strings.ToLower(strings.TrimSpace(x.Status))
	x.Challenge = strings.ToLower(strings.TrimSpace(x.Challenge))
	allowed := map[string]bool{"not_tested": true, "login_required": true, "challenge_required": true, "authenticated": true, "failed": true}
	ch := map[string]bool{"none": true, "captcha": true, "two_factor": true, "reauth": true}
	if !allowed[x.Status] || !ch[x.Challenge] {
		writeJSON(w, 400, map[string]string{"error": "invalid_login_state"})
		return
	}
	_, err := s.DB.ExecContext(r.Context(), `UPDATE accounts SET browser_login_status=$2,browser_login_detail=NULLIF($3,''),browser_login_checked_at=now(),browser_challenge_type=$4,browser_challenge_at=CASE WHEN $4='none' THEN NULL ELSE now() END,browser_challenge_detail=CASE WHEN $4='none' THEN NULL ELSE NULLIF($3,'') END,updated_at=now() WHERE id=$1`, id, x.Status, x.Detail, x.Challenge)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"status": x.Status, "challenge": x.Challenge})
}
func (s *Server) requestPasswordRotation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var configured bool
	_ = s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(login_email,'')<>'' AND COALESCE(login_password_secret_ref,'')<>'' FROM accounts WHERE id=$1`, id).Scan(&configured)
	if !configured {
		writeJSON(w, 409, map[string]string{"error": "login_credentials_not_configured"})
		return
	}
	_, err := s.DB.ExecContext(r.Context(), `UPDATE accounts SET password_rotation_status='pending',password_rotation_detail='waiting for authenticated web session',password_rotation_requested_at=now(),updated_at=now() WHERE id=$1`, id)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 202, map[string]string{"status": "pending"})
}
