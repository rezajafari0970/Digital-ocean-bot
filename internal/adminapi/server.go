package adminapi

import (
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/observability"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/http"
	"os"
	"sync"
	"time"
)

type Server struct {
	WebPath             string
	Auth                auth.Service
	DB                  *sql.DB
	Container           app.Container
	Health              observability.Health
	LoginLimiter        *LoginLimiter
	OutputRuntimes      *sanaei.RuntimeManager
	OutputPauseMu       sync.RWMutex
	OutputRefreshMu     sync.Mutex
	OutputPanelMu       sync.Mutex
	OutputPanelRun      map[string]bool
	OutputRefreshSem    chan struct{}
	OutputRefreshCursor int
	BrowserMu           sync.Mutex
	BrowserTickets      map[string]time.Time
}

func New(db *sql.DB, c app.Container) *Server {
	return &Server{WebPath: "/admin", DB: db, Container: c, Health: observability.Health{DB: db, RequireWorker: true, WorkerMode: os.Getenv("DOB_WORKER_MODE")}, Auth: auth.Service{Store: auth.SQLStore{DB: db}}, LoginLimiter: NewLoginLimiter(), OutputRuntimes: &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: c.Secrets, Timeout: 90 * time.Second}, TTL: 2 * time.Minute}, OutputPanelRun: map[string]bool{}, OutputRefreshSem: make(chan struct{}, 64), BrowserTickets: map[string]time.Time{}}
}
func (s *Server) Routes() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/v1/proxy-traffic", s.require(s.proxyTraffic, false))
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
	m.HandleFunc("GET /version", s.version)
	m.HandleFunc("POST /api/v1/auth/login", s.login)
	m.HandleFunc("POST /api/v1/auth/logout", s.require(s.logout, false))
	m.HandleFunc("POST /api/v1/accounts", s.require(s.createAccount, true))
	m.HandleFunc("POST /api/v1/accounts/preview", s.require(s.accountPreview, true))
	m.HandleFunc("PUT /api/v1/accounts/{id}", s.require(s.updateAccount, true))
	m.HandleFunc("DELETE /api/v1/accounts/{id}", s.require(s.deleteAccount, true))
	m.HandleFunc("POST /api/v1/accounts/{id}/purge", s.require(s.purgeAccount, true))
	m.HandleFunc("GET /api/v1/accounts/{id}/options", s.require(s.accountOptions, false))
	m.HandleFunc("POST /api/v1/accounts/{id}/identity", s.require(s.accountIdentity, true))
	m.HandleFunc("POST /api/v1/accounts/{id}/create-block/release", s.require(s.releaseAccountCreateBlock, true))
	m.HandleFunc("POST /api/v1/accounts/{id}/upcloud-trial", s.require(s.enableUpCloudTrial, true))
	m.HandleFunc("POST /api/v1/accounts/{id}/preflight", s.require(s.accountPreflight, true))
	m.HandleFunc("POST /api/v1/accounts/{id}/console-session", s.require(s.createVultrBrowserTicket, true))
	m.HandleFunc("GET /vultr-browser/{path...}", s.vultrBrowserProxy)
	m.HandleFunc("POST /vultr-browser/input/scroll", s.vultrBrowserProxy)
	m.HandleFunc("POST /vultr-browser/input/tap", s.vultrBrowserProxy)
	m.HandleFunc("GET /websockify", s.vultrBrowserProxy)
	m.HandleFunc("POST /api/v1/proxies", s.require(s.createProxy, true))
	m.HandleFunc("PUT /api/v1/proxies/{id}", s.require(s.updateProxy, true))
	m.HandleFunc("DELETE /api/v1/proxies/{id}", s.require(s.deleteProxy, true))
	m.HandleFunc("GET /api/v1/proxies/{id}", s.require(s.proxyDetails, false))
	m.HandleFunc("GET /api/v1/residential-performance", s.require(s.residentialPerformanceStatus, false))
	m.HandleFunc("POST /api/v1/residential-performance", s.require(s.residentialPerformanceAction, true))
	m.HandleFunc("GET /api/v1/residential-routing", s.require(s.residentialRoutingStatus, false))
	m.HandleFunc("GET /api/v1/residential-proxies", s.require(s.residentialProxies, false))
	m.HandleFunc("POST /api/v1/residential-proxies", s.require(s.createResidentialProxy, true))
	m.HandleFunc("POST /api/v1/residential-proxies/import", s.require(s.importResidentialProxies, true))
	m.HandleFunc("POST /api/v1/residential-proxies/delete", s.require(s.deleteResidentialProxies, true))
	m.HandleFunc("POST /api/v1/residential-proxies/export", s.require(s.exportResidentialProxies, true))
	m.HandleFunc("PUT /api/v1/residential-proxies/{id}", s.require(s.updateResidentialProxy, true))
	m.HandleFunc("DELETE /api/v1/residential-proxies/{id}", s.require(s.deleteResidentialProxy, true))
	m.HandleFunc("POST /api/v1/residential-proxies/{id}/test", s.require(s.testResidentialProxy, true))
	m.HandleFunc("GET /api/v1/configs", s.require(s.getGlobalConfigs, false))
	m.HandleFunc("GET /api/v1/configs/dns", s.require(s.getDNSCatalog, false))
	m.HandleFunc("GET /api/v1/server-protection", s.require(s.serverProtectionStatus, false))
	m.HandleFunc("POST /api/v1/server-protection", s.require(s.serverProtectionAction, true))
	m.HandleFunc("GET /api/v1/config-capacity", s.require(s.configCapacity, false))
	m.HandleFunc("POST /api/v1/config-capacity/delete-all-clients", s.require(s.deleteAllCapacityClients, true))
	m.HandleFunc("POST /api/v1/config-capacity/resume", s.require(s.resumeCapacityAutomation, true))
	m.HandleFunc("POST /api/v1/config-capacity/restore", s.require(s.restoreCapacityAutomation, true))
	m.HandleFunc("GET /api/v1/config-capacity/cleanup/current", s.require(s.currentCleanupJob, false))
	m.HandleFunc("POST /api/v1/config-capacity/cleanup/{id}/cancel", s.require(s.cancelCleanupJob, true))
	m.HandleFunc("POST /api/v1/config-capacity/cleanup/{id}/resume", s.require(s.resumeCleanupJob, true))
	m.HandleFunc("GET /api/v1/config-capacity/cleanup/{id}", s.require(s.cleanupJobStatus, false))
	m.HandleFunc("GET /api/v1/output", s.require(s.outputConfigs, false))
	m.HandleFunc("POST /api/v1/output/share", s.require(s.createOutputShare, true))
	m.HandleFunc("GET /share/output/{token}", s.sharedOutput)
	m.HandleFunc("GET /api/v1/config-panels", s.require(s.getConfigPanels, false))
	m.HandleFunc("GET /api/v1/client-mutations", s.require(s.listClientMutations, false))
	m.HandleFunc("POST /api/v1/client-mutations", s.require(s.clientMutations, true))
	m.HandleFunc("PATCH /api/v1/client-mutations", s.require(s.clientMutations, true))
	m.HandleFunc("DELETE /api/v1/client-mutations", s.require(s.clientMutations, true))
	m.HandleFunc("GET /api/v1/client-mutations/{id}", s.require(s.clientMutationStatus, false))
	m.HandleFunc("PUT /api/v1/configs", s.require(s.putGlobalConfig, true))
	m.HandleFunc("POST /api/v1/proxies/{id}/test", s.require(s.testProxy, true))
	m.HandleFunc("PUT /api/v1/accounts/{id}/network", s.require(s.assignProxy, true))
	m.HandleFunc("GET /api/v1/accounts/{id}/discovery", s.require(s.accountDiscovery, false))
	m.HandleFunc("POST /api/v1/accounts/{id}/refresh", s.require(s.accountDiscovery, true))
	m.HandleFunc("POST /api/v1/accounts/{id}/proxy-test", s.require(s.testAccountProxy, true))
	m.HandleFunc("GET /api/v1/templates", s.require(s.listTemplates, false))
	m.HandleFunc("POST /api/v1/templates", s.require(s.uploadTemplate, true))
	m.HandleFunc("GET /api/v1/accounts/{id}/dashboard", s.require(s.accountDashboard, false))
	m.HandleFunc("GET /api/v1/accounts/{id}/resources", s.require(s.accountResources, false))
	m.HandleFunc("GET /api/v1/accounts/{id}/capacity", s.require(s.accountCapacity, false))
	m.HandleFunc("GET /api/v1/accounts/{id}/runtime", s.require(s.accountRuntime, false))
	m.HandleFunc("GET /api/v1/accounts", s.require(s.accounts, false))
	m.HandleFunc("GET /api/v1/proxies", s.require(s.proxies, false))
	m.HandleFunc("GET /api/v1/build-activity", s.require(s.buildActivity, false))
	m.HandleFunc("GET /api/v1/build-activity/{id}/provision", s.require(s.provisionActivity, false))
	m.HandleFunc("GET /api/v1/provision-errors", s.require(s.provisionErrors, false))
	m.HandleFunc("POST /api/v1/installers/sanaei/preview", s.require(s.previewSanaeiInstaller, true))
	m.HandleFunc("POST /api/v1/installers/sanaei", s.require(s.createSanaeiInstaller, true))
	m.HandleFunc("GET /api/v1/installers", s.require(s.listInstallers, false))
	m.HandleFunc("POST /api/v1/installers", s.require(s.createInstaller, true))
	m.HandleFunc("GET /api/v1/install-scripts", s.require(s.listInstallScripts, false))
	m.HandleFunc("POST /api/v1/install-scripts", s.require(s.createInstallScript, true))
	m.HandleFunc("POST /api/v1/deployments/{id}/installer", s.require(s.selectDeploymentInstaller, true))
	m.HandleFunc("POST /api/v1/deployments/{id}/installer/rearm", s.require(s.rearmDeploymentInstaller, true))
	m.HandleFunc("GET /api/v1/audit", s.require(s.audit, false))
	m.HandleFunc("GET /api/v1/system", s.require(s.system, false))
	m.HandleFunc("GET /api/v1/providers", s.require(s.providersMetadata, false))
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

// The retired interactive browser is not part of account API management.
func (s *Server) vultrBrowserProxy(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusGone, map[string]string{"error": "vultr_console_retired"})
}
func (s *Server) createVultrBrowserTicket(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusGone, map[string]string{"error": "vultr_console_retired"})
}
