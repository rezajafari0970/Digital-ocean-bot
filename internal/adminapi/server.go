package adminapi

import (
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/observability"
	"net/http"
)

type Server struct {
	WebPath      string
	Auth         auth.Service
	DB           *sql.DB
	Container    app.Container
	Health       observability.Health
	LoginLimiter *LoginLimiter
}

func New(db *sql.DB, c app.Container) *Server {
	return &Server{WebPath: "/admin", DB: db, Container: c, Health: observability.Health{DB: db}, Auth: auth.Service{Store: auth.SQLStore{DB: db}}, LoginLimiter: NewLoginLimiter()}
}
func (s *Server) Routes() *http.ServeMux {
	m := http.NewServeMux()
	m.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	m.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != s.WebPath && r.URL.Path != s.WebPath+"/" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "web/static/index.html")
	})
	m.HandleFunc("GET /healthz", s.health)
	m.HandleFunc("GET /readyz", s.ready)
	m.HandleFunc("POST /api/v1/auth/login", s.login)
	m.HandleFunc("POST /api/v1/auth/logout", s.require(s.logout, false))
	m.HandleFunc("POST /api/v1/accounts", s.require(s.createAccount, true))
	m.HandleFunc("POST /api/v1/accounts/preview", s.require(s.accountPreview, true))
	m.HandleFunc("PUT /api/v1/accounts/{id}", s.require(s.updateAccount, true))
	m.HandleFunc("DELETE /api/v1/accounts/{id}", s.require(s.deleteAccount, true))
	m.HandleFunc("GET /api/v1/accounts/{id}/options", s.require(s.accountOptions, false))
	m.HandleFunc("POST /api/v1/accounts/{id}/identity", s.require(s.accountIdentity, true))
	m.HandleFunc("POST /api/v1/accounts/{id}/preflight", s.require(s.accountPreflight, true))
	m.HandleFunc("POST /api/v1/proxies", s.require(s.createProxy, true))
	m.HandleFunc("PUT /api/v1/proxies/{id}", s.require(s.updateProxy, true))
	m.HandleFunc("DELETE /api/v1/proxies/{id}", s.require(s.deleteProxy, true))
	m.HandleFunc("GET /api/v1/proxies/{id}", s.require(s.proxyDetails, false))
	m.HandleFunc("POST /api/v1/proxies/{id}/test", s.require(s.testProxy, true))
	m.HandleFunc("PUT /api/v1/accounts/{id}/network", s.require(s.assignProxy, true))
	m.HandleFunc("POST /api/v1/profiles", s.require(s.createProfile, true))
	m.HandleFunc("PUT /api/v1/traffic-policy", s.require(s.upsertTrafficPolicy, true))
	m.HandleFunc("GET /api/v1/accounts/{id}/discovery", s.require(s.accountDiscovery, false))
	m.HandleFunc("POST /api/v1/accounts/{id}/refresh", s.require(s.accountDiscovery, true))
	m.HandleFunc("POST /api/v1/accounts/{id}/proxy-test", s.require(s.testAccountProxy, true))
	m.HandleFunc("POST /api/v1/templates", s.require(s.uploadTemplate, true))
	m.HandleFunc("GET /api/v1/accounts/{id}/dashboard", s.require(s.accountDashboard, false))
	m.HandleFunc("POST /api/v1/accounts/{id}/browser-identity", s.require(s.browserIdentity, true))
	m.HandleFunc("POST /api/v1/accounts/{id}/browser-audit", s.require(s.runAssignedBrowserAudit, true))
	m.HandleFunc("POST /api/v1/accounts/{id}/browser-geo-audit", s.require(s.runGeoBrowserAudit, true))
	m.HandleFunc("POST /api/v1/accounts/{id}/login-state", s.require(s.setLoginState, true))
	m.HandleFunc("PUT /api/v1/accounts/{id}/login-credentials", s.require(s.updateLoginCredentials, true))
	m.HandleFunc("POST /api/v1/accounts/{id}/password-rotation/request", s.require(s.requestPasswordRotation, true))
	m.HandleFunc("GET /api/v1/accounts/{id}/resources", s.require(s.accountResources, false))
	m.HandleFunc("GET /api/v1/accounts/{id}/capacity", s.require(s.accountCapacity, false))
	m.HandleFunc("GET /api/v1/accounts/{id}/runtime", s.require(s.accountRuntime, false))
	m.HandleFunc("GET /api/v1/schedules", s.require(s.listSchedules, false))
	m.HandleFunc("PUT /api/v1/schedules", s.require(s.upsertSchedule, true))
	m.HandleFunc("GET /api/v1/accounts", s.require(s.accounts, false))
	m.HandleFunc("GET /api/v1/proxies", s.require(s.proxies, false))
	m.HandleFunc("GET /api/v1/build-activity", s.require(s.buildActivity, false))
	m.HandleFunc("GET /api/v1/build-activity/{id}/provision", s.require(s.provisionActivity, false))
	m.HandleFunc("GET /api/v1/provision-errors", s.require(s.provisionErrors, false))
	m.HandleFunc("GET /api/v1/installers", s.require(s.listInstallers, false))
	m.HandleFunc("POST /api/v1/installers", s.require(s.createInstaller, true))
	m.HandleFunc("GET /api/v1/install-scripts", s.require(s.listInstallScripts, false))
	m.HandleFunc("POST /api/v1/install-scripts", s.require(s.createInstallScript, true))
	m.HandleFunc("GET /api/v1/profiles", s.require(s.profiles, false))
	m.HandleFunc("POST /api/v1/deployments", s.require(s.createDeployment, true))
	m.HandleFunc("GET /api/v1/deployments", s.require(s.deployments, false))
	m.HandleFunc("POST /api/v1/deployments/{id}/installer", s.require(s.selectDeploymentInstaller, true))
	m.HandleFunc("GET /api/v1/audit", s.require(s.audit, false))
	m.HandleFunc("GET /api/v1/system", s.require(s.system, false))
	m.HandleFunc("GET /api/v1/browser-runtimes", s.require(s.browserRuntimes, false))
	m.HandleFunc("GET /api/v1/panel-settings", s.require(s.getPanelSettings, false))
	m.HandleFunc("PUT /api/v1/panel-settings", s.require(s.updatePanelSettings, true))
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
