package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"io"
	"net/http"
	"strings"
	"time"
)

type principalKey struct{}
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.LoginLimiter != nil && !s.LoginLimiter.Allow(r.RemoteAddr) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too_many_attempts"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req loginRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	authSvc := s.Auth
	if req.Remember {
		authSvc.TTL = 30 * 24 * time.Hour
	}
	token, p, err := authSvc.Login(r.Context(), req.Username, req.Password)
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

func (s *Server) authMe(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	writeJSON(w, 200, p)
}
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	if s.LoginLimiter != nil && !s.LoginLimiter.Allow(r.RemoteAddr) {
		writeJSON(w, 429, map[string]string{"error": "too_many_attempts"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		Current string `json:"current_password"`
		Next    string `json:"new_password"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || dec.Decode(new(any)) != io.EOF || len(req.Next) < 12 || len(req.Next) > 72 {
		writeJSON(w, 400, map[string]string{"error": "password_must_be_12_to_72_bytes"})
		return
	}
	p, _ := principal(r.Context())
	err := (auth.SQLStore{DB: s.DB}).ChangePassword(r.Context(), p.UserID, req.Current, req.Next)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		writeJSON(w, 400, map[string]string{"error": "invalid_current_or_new_password"})
		return
	}
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
