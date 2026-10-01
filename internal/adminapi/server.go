package adminapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/observability"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Server struct {
	WebPath          string
	Auth             auth.Service
	DB               *sql.DB
	Container        app.Container
	Health           observability.Health
	LoginLimiter     *LoginLimiter
	OutputRuntimes   *sanaei.RuntimeManager
	OutputCacheMu    sync.RWMutex
	OutputCache      map[string]outputCacheEntry
	OutputRefreshMu  sync.Mutex
	OutputRefreshing map[string]bool
	OutputContext    context.Context
	OutputPauseMu    sync.RWMutex
	CleanupMu        sync.RWMutex
	CleanupJobs      map[string]*cleanupJob
	BrowserMu        sync.Mutex
	BrowserTickets   map[string]time.Time
}

func New(db *sql.DB, c app.Container) *Server {
	return &Server{WebPath: "/admin", DB: db, Container: c, Health: observability.Health{DB: db}, Auth: auth.Service{Store: auth.SQLStore{DB: db}}, LoginLimiter: NewLoginLimiter(), OutputRuntimes: &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: c.Secrets, Timeout: 90 * time.Second}, TTL: 2 * time.Minute}, OutputCache: map[string]outputCacheEntry{}, OutputRefreshing: map[string]bool{}, OutputContext: context.Background(), CleanupJobs: map[string]*cleanupJob{}, BrowserTickets: map[string]time.Time{}}
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
	m.HandleFunc("GET /version", s.version)
	m.HandleFunc("POST /api/v1/auth/login", s.login)
	m.HandleFunc("POST /api/v1/auth/logout", s.require(s.logout, false))
	m.HandleFunc("POST /api/v1/accounts", s.require(s.createAccount, true))
	m.HandleFunc("POST /api/v1/accounts/preview", s.require(s.accountPreview, true))
	m.HandleFunc("PUT /api/v1/accounts/{id}", s.require(s.updateAccount, true))
	m.HandleFunc("DELETE /api/v1/accounts/{id}", s.require(s.deleteAccount, true))
	m.HandleFunc("GET /api/v1/accounts/{id}/options", s.require(s.accountOptions, false))
	m.HandleFunc("POST /api/v1/accounts/{id}/identity", s.require(s.accountIdentity, true))
	m.HandleFunc("POST /api/v1/accounts/{id}/preflight", s.require(s.accountPreflight, true))
	m.HandleFunc("POST /api/v1/accounts/{id}/console-session", s.require(s.createVultrBrowserTicket, true))
	m.HandleFunc("GET /vultr-browser/{path...}", s.vultrBrowserProxy)
	m.HandleFunc("GET /websockify", s.vultrBrowserProxy)
	m.HandleFunc("POST /api/v1/proxies", s.require(s.createProxy, true))
	m.HandleFunc("PUT /api/v1/proxies/{id}", s.require(s.updateProxy, true))
	m.HandleFunc("DELETE /api/v1/proxies/{id}", s.require(s.deleteProxy, true))
	m.HandleFunc("GET /api/v1/proxies/{id}", s.require(s.proxyDetails, false))
	m.HandleFunc("GET /api/v1/residential-proxies", s.require(s.residentialProxies, false))
	m.HandleFunc("POST /api/v1/residential-proxies", s.require(s.createResidentialProxy, true))
	m.HandleFunc("PUT /api/v1/residential-proxies/{id}", s.require(s.updateResidentialProxy, true))
	m.HandleFunc("DELETE /api/v1/residential-proxies/{id}", s.require(s.deleteResidentialProxy, true))
	m.HandleFunc("POST /api/v1/residential-proxies/{id}/test", s.require(s.testResidentialProxy, true))
	m.HandleFunc("GET /api/v1/configs", s.require(s.getGlobalConfigs, false))
	m.HandleFunc("GET /api/v1/config-capacity", s.require(s.configCapacity, false))
	m.HandleFunc("POST /api/v1/config-capacity/delete-all-clients", s.require(s.deleteAllCapacityClients, true))
	m.HandleFunc("GET /api/v1/config-capacity/cleanup/{id}", s.require(s.cleanupJobStatus, false))
	m.HandleFunc("GET /api/v1/output", s.require(s.outputConfigs, false))
	m.HandleFunc("POST /api/v1/output/share", s.require(s.createOutputShare, true))
	m.HandleFunc("GET /share/output/{token}", s.sharedOutput)
	m.HandleFunc("GET /api/v1/config-panels", s.require(s.getConfigPanels, false))
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

func (s *Server) vultrBrowserProxy(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/websockify" {
		_, cookieErr := r.Cookie("vultr_browser_session")
		log.Printf("vultr ws request upgrade=%q connection=%q protocol=%q cookie=%t", r.Header.Get("Upgrade"), r.Header.Get("Connection"), r.Header.Get("Sec-WebSocket-Protocol"), cookieErr == nil)
	}
	c, err := r.Cookie("vultr_browser_session")
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	s.BrowserMu.Lock()
	exp, ok := s.BrowserTickets[c.Value]
	if !ok || time.Now().After(exp) {
		delete(s.BrowserTickets, c.Value)
		ok = false
	}
	s.BrowserMu.Unlock()
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/vultr-browser/client.html" || r.URL.Path == "/vultr-browser/vnc.html" || r.URL.Path == "/vultr-browser/vnc_lite.html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte("<!doctype html><html><head><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><link rel=\"stylesheet\" href=\"/static/vultr-console.css?v=7\"></head><body><div id=\"status\">Starting...</div><div id=\"screen\"></div><script type=\"module\" src=\"/static/vultr-console.js?v=7\"></script></body></html>"))
		return
	}
	upstream := "http://127.0.0.1:16080"
	if r.URL.Path == "/websockify" {
		upstream = "http://127.0.0.1:15900"
	}
	target, _ := url.Parse(upstream)
	proxy := httputil.NewSingleHostReverseProxy(target)
	if strings.HasPrefix(r.URL.Path, "/vultr-browser") {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/vultr-browser")
		if r.URL.Path == "" || r.URL.Path == "/" {
			r.URL.Path = "/vnc.html"
			q := r.URL.Query()
			q.Set("autoconnect", "1")
			q.Set("resize", "scale")
			q.Set("path", "websockify")
			r.URL.RawQuery = q.Encode()
		}
	}
	proxy.ServeHTTP(w, r)
}

func (s *Server) ensureVultrBrowser(accountID string) error {
	c, err := net.DialTimeout("unix", "/run/digital-ocean-bot/vultr-browser.sock", time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	if _, err = c.Write([]byte(accountID + "\n")); err != nil {
		return err
	}
	buf := make([]byte, 256)
	n, err := c.Read(buf)
	if err != nil {
		return err
	}
	response := strings.TrimSpace(string(buf[:n]))
	if response != "READY" {
		return errors.New(response)
	}
	return nil
}

func (s *Server) createVultrBrowserTicket(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	var provider string
	if err := s.DB.QueryRowContext(r.Context(), "SELECT provider FROM accounts WHERE id=$1", accountID).Scan(&provider); err != nil || provider != "vultr" {
		writeJSON(w, 404, map[string]string{"error": "vultr_account_not_found"})
		return
	}
	if err := s.ensureVultrBrowser(accountID); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "vultr_console_unavailable", "detail": err.Error()})
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	token := hex.EncodeToString(raw)
	s.BrowserMu.Lock()
	for k, exp := range s.BrowserTickets {
		if time.Now().After(exp) {
			delete(s.BrowserTickets, k)
		}
	}
	s.BrowserTickets[token] = time.Now().Add(5 * time.Minute)
	s.BrowserMu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "vultr_browser_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 300})
	writeJSON(w, 200, map[string]any{"ok": true, "expires_in": 300})
}
