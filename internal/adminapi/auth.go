package adminapi

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"net/http"
	"strings"
)

type principalKey struct{}
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	token, p, err := s.Auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "invalid_credentials"})
		return
	}
	writeJSON(w, 200, map[string]any{"token": token, "user": p})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r.Context())
	if !ok {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	_ = s.Auth.Store.DeleteSession(r.Context(), p.SessionID)
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) require(next http.HandlerFunc, write bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		p, err := s.Auth.Authenticate(r.Context(), token)
		if err != nil {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		if write && !p.CanWrite() {
			writeJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	}
}
func principal(ctx context.Context) (auth.Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(auth.Principal)
	return p, ok
}
