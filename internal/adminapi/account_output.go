package adminapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
)

var accountOutputID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type accountOutputScope struct {
	AccountID string
	Token     string
}

func accountOutputHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func (s *Server) createAccountOutputShare(w http.ResponseWriter, r *http.Request) {
	accountOutputHeaders(w)
	id := r.PathValue("id")
	if !accountOutputID.MatchString(id) {
		writeJSON(w, 400, map[string]string{"error": "invalid_account_id"})
		return
	}
	var body struct {
		RouteClass string `json:"route_class"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || (body.RouteClass != "DIRECT" && body.RouteClass != "RESIDENTIAL") {
		writeJSON(w, 400, map[string]string{"error": "invalid_route_class"})
		return
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	token := newShareToken()
	if token == "" {
		writeJSON(w, 500, errorBody())
		return
	}
	var created string
	err := s.DB.QueryRowContext(r.Context(), `INSERT INTO account_output_share_tokens(token,account_id,route_class)
 SELECT $1,id,$3 FROM accounts WHERE id=$2 AND deleted_at IS NULL AND deletion_requested_at IS NULL RETURNING token`, token, id, body.RouteClass).Scan(&created)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]string{"token": created, "url": "/share/account-output/" + created})
}

func (s *Server) sharedAccountOutput(w http.ResponseWriter, r *http.Request) {
	accountOutputHeaders(w)
	// Scope comes exclusively from the persisted capability, never from URL filters.
	if q := r.URL.Query(); q.Has("account_id") || q.Has("route_class") {
		http.Error(w, "account output scope is fixed", 400)
		return
	}
	scope := accountOutputScope{Token: r.PathValue("token")}
	var class string
	err := s.DB.QueryRowContext(r.Context(), `SELECT t.account_id::text,t.route_class
 FROM account_output_share_tokens t JOIN accounts a ON a.id=t.account_id
 WHERE t.token=$1 AND t.revoked_at IS NULL AND a.deleted_at IS NULL AND a.deletion_requested_at IS NULL`, scope.Token).Scan(&scope.AccountID, &class)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if class != "DIRECT" && class != "RESIDENTIAL" {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("view") == "1" {
		outputBrowserView(w)
		return
	}
	s.outputSnapshotScoped(w, r, class, &scope)
}
