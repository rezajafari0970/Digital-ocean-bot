package adminapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

type loginCredentialsWrite struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) updateLoginCredentials(
	w http.ResponseWriter,
	r *http.Request,
) {
	id := r.PathValue("id")

	var x loginCredentialsWrite

	if json.NewDecoder(r.Body).Decode(&x) != nil {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{"error": "invalid_request"},
		)
		return
	}

	x.Email = strings.TrimSpace(x.Email)

	var currentEmail, currentRef string

	err := s.DB.QueryRowContext(
		r.Context(),
		`SELECT
COALESCE(login_email,''),
COALESCE(login_password_secret_ref,'')
 FROM accounts
 WHERE id=$1`,
		id,
	).Scan(
		&currentEmail,
		&currentRef,
	)

	if err != nil {
		writeJSON(
			w,
			http.StatusNotFound,
			map[string]string{"error": "account_not_found"},
		)
		return
	}

	// Empty email means "keep existing email".
	if x.Email == "" {
		x.Email = currentEmail
	}

	if x.Email == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"error": "login_email_required",
			},
		)
		return
	}

	secretRef := currentRef

	// Password is write-only.
	// Empty password means preserve the existing secret.
	if x.Password != "" {
		secretRef = "do-login-password"

		if err := s.Container.Secrets.Put(
			r.Context(),
			id,
			secretRef,
			"digitalocean_login_password",
			[]byte(x.Password),
		); err != nil {
			writeJSON(
				w,
				http.StatusInternalServerError,
				map[string]string{
					"error": "login_password_store_failed",
				},
			)
			return
		}
	}

	if secretRef == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"error": "login_password_required",
			},
		)
		return
	}

	_, err = s.DB.ExecContext(
		r.Context(),
		`UPDATE accounts
 SET
login_email=$2,
login_password_secret_ref=$3,
browser_login_status='not_tested',
browser_login_detail=NULL,
browser_login_checked_at=NULL,
browser_challenge_type='none',
browser_challenge_at=NULL,
browser_challenge_detail=NULL,
password_rotation_status='pending',
password_rotation_detail='credentials configured',
updated_at=now()
 WHERE id=$1`,
		id,
		x.Email,
		secretRef,
	)

	if err != nil {
		writeJSON(
			w,
			http.StatusInternalServerError,
			errorBody(),
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"configured": true,
			"email":      x.Email,
		},
	)
}
