package adminapi

import (
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/observability"
	"net/http"
)

type Server struct {
	DB        *sql.DB
	Container app.Container
	Health    observability.Health
}

func New(db *sql.DB, c app.Container) *Server {
	return &Server{DB: db, Container: c, Health: observability.Health{DB: db}}
}
func (s *Server) Routes() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", s.health)
	m.HandleFunc("GET /readyz", s.ready)
	m.HandleFunc("GET /api/v1/accounts", s.accounts)
	m.HandleFunc("GET /api/v1/proxies", s.proxies)
	m.HandleFunc("GET /api/v1/profiles", s.profiles)
	m.HandleFunc("POST /api/v1/deployments", s.createDeployment)
	m.HandleFunc("GET /api/v1/deployments", s.deployments)
	m.HandleFunc("GET /api/v1/audit", s.audit)
	m.HandleFunc("GET /api/v1/system", s.system)
	return m
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	x := s.Health.Readiness(r.Context())
	code := 200
	if x.Status != "ready" {
		code = 503
	}
	writeJSON(w, code, x)
}
