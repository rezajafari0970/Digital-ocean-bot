package adminapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/geoctx"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providerbrowser"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ovh"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxyselection"
)

type providerBrowserSession struct {
	owner, login, provider, account string
	authorization                   string
	peer                            *providerbrowser.Peer
	route                           *app.OnboardingRoute
	cancel                          context.CancelFunc
	expires                         time.Time
	mu                              sync.Mutex
	lastUsed                        time.Time
	once                            sync.Once
}

func (b *providerBrowserSession) close() {
	b.once.Do(func() {
		b.cancel()
		if b.peer != nil {
			b.peer.Close()
		}
		if b.route != nil {
			b.route.Close()
		}
	})
}
func (b *providerBrowserSession) touch() { b.mu.Lock(); b.lastUsed = time.Now(); b.mu.Unlock() }
func (s *Server) closeProviderBrowsers(login string) {
	s.BrowserMu.Lock()
	defer s.BrowserMu.Unlock()
	for id, b := range s.ProviderBrowsers {
		if b.login == login {
			delete(s.ProviderBrowsers, id)
			b.close()
		}
	}
}
func (s *Server) browserSession(w http.ResponseWriter, r *http.Request) *providerBrowserSession {
	if !authorizationAdmin(w, r) {
		return nil
	}
	p, _ := principal(r.Context())
	s.BrowserMu.Lock()
	b := s.ProviderBrowsers[r.PathValue("id")]
	s.BrowserMu.Unlock()
	if b == nil || b.owner != p.UserID || b.login != p.SessionID || !b.expires.After(time.Now()) {
		writeJSON(w, 404, map[string]string{"error": "browser_session_unavailable"})
		return nil
	}
	select {
	case <-b.peer.Done():
		writeJSON(w, 410, map[string]string{"error": "browser_session_closed", "detail": "The isolated session ended. Open a new session to verify the route again."})
		return nil
	default:
	}
	b.touch()
	return b
}
func (s *Server) startProviderBrowser(w http.ResponseWriter, r *http.Request) {
	if !authorizationAdmin(w, r) {
		return
	}
	p, _ := principal(r.Context())
	if p.SessionID == "" {
		writeJSON(w, 403, errorBody())
		return
	}
	var req struct {
		Provider        string   `json:"provider"`
		AccountID       string   `json:"account_id"`
		AuthorizationID string   `json:"authorization_id"`
		ProxyID         string   `json:"proxy_id"`
		ProxyIDs        []string `json:"proxy_ids"`
		ProxyCountry    string   `json:"proxy_country"`
		ProxyCountries  []string `json:"proxy_countries"`
		Width           int64    `json:"width"`
		Height          int64    `json:"height"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || dec.Decode(new(any)) != io.EOF {
		writeJSON(w, 400, errorBody())
		return
	}
	if req.AccountID != "" {
		var provider string
		if s.DB.QueryRowContext(r.Context(), `SELECT provider FROM accounts WHERE id=$1`, req.AccountID).Scan(&provider) != nil || provider != req.Provider {
			writeJSON(w, 409, map[string]string{"error": "account_provider_changed"})
			return
		}
	}
	if s.Container.Providers == nil {
		writeJSON(w, 400, errorBody())
		return
	}
	meta, ok := s.Container.Providers.Metadata(req.Provider)
	if !ok || !providerbrowser.ValidURL(meta.WebsiteURL) {
		writeJSON(w, 400, map[string]string{"error": "provider_website_unavailable"})
		return
	}
	countries, e := proxyselection.Priorities(req.ProxyCountry, req.ProxyCountries)
	ids := normalizeProxyPool(req.ProxyID, req.ProxyIDs)
	if e != nil || req.ProxyID == "" || len(ids) > 3 {
		writeJSON(w, 400, map[string]string{"error": "browser_proxy_required", "detail": "Select an eligible proxy and up to three country priorities."})
		return
	}
	if req.Width < 360 || req.Width > 1440 || req.Height < 500 || req.Height > 1200 {
		writeJSON(w, 400, map[string]string{"error": "invalid_browser_viewport"})
		return
	}
	startURL := meta.WebsiteURL
	if req.AuthorizationID != "" {
		flow, err := s.loadAuthorization(r.Context(), p.UserID, req.AuthorizationID)
		if err != nil || req.AccountID != "" || flow.Provider != "ovh" || flow.Provider != req.Provider || flow.State != "PENDING" || !sameAuthorizationPolicy(flow, ids, countries) {
			writeJSON(w, 409, map[string]string{"error": "authorization_browser_binding_changed"})
			return
		}
		secret, err := s.authorizationSecret(flow)
		var scope struct {
			Endpoint string `json:"endpoint"`
		}
		if err != nil || json.Unmarshal([]byte(secret.Token), &scope) != nil || !ovh.ValidAuthorizationURL(secret.ValidationURL, scope.Endpoint) {
			writeJSON(w, 409, map[string]string{"error": "authorization_browser_unavailable"})
			return
		}
		startURL = secret.ValidationURL
		ids = normalizeProxyPool(flow.RouteProxyID, nil)
		countries = []string{flow.RouteCountry}
	}
	var token [24]byte
	if _, e = rand.Read(token[:]); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	id := hex.EncodeToString(token[:])
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	b := &providerBrowserSession{owner: p.UserID, login: p.SessionID, provider: req.Provider, account: req.AccountID, authorization: req.AuthorizationID, cancel: cancel, expires: time.Now().Add(15 * time.Minute), lastUsed: time.Now()}
	s.BrowserMu.Lock()
	if s.ProviderBrowsers == nil {
		s.ProviderBrowsers = map[string]*providerBrowserSession{}
	}
	if len(s.ProviderBrowsers)+s.BrowserStarting >= 2 {
		s.BrowserMu.Unlock()
		cancel()
		writeJSON(w, 429, map[string]string{"error": "browser_capacity_full", "detail": "Close an existing provider browser before opening another."})
		return
	}
	// Reserve capacity through startup as well, without holding the map mutex.
	s.BrowserStarting++
	s.BrowserMu.Unlock()
	defer func() { s.BrowserMu.Lock(); s.BrowserStarting--; s.BrowserMu.Unlock() }()
	published := false
	defer func() {
		if !published {
			b.close()
		}
	}()
	// Stop preparation if the caller leaves, then detach only after it is ready.
	stopRequest := context.AfterFunc(r.Context(), cancel)
	defer stopRequest()
	container := s.Container
	container.DB = s.DB
	route, e := container.OnboardingPriorities(ctx, ids, countries, true)
	if e != nil {
		writeJSON(w, 409, map[string]string{"error": "browser_route_unverified", "detail": e.Error()})
		return
	}
	b.route = route
	dial := net.Dialer{Timeout: 3 * time.Second}
	conn, e := dial.DialContext(ctx, "unix", "/run/datacenter-provider-browser.sock")
	if e != nil {
		writeJSON(w, 503, map[string]string{"error": "browser_sandbox_unavailable", "detail": "The isolated browser service is unavailable. No website was opened."})
		return
	}
	b.peer = providerbrowser.NewPeer(ctx, conn)
	b.peer.Dial = func(dialCtx context.Context, address string) (net.Conn, error) {
		conn, err := route.Dial(dialCtx, address)
		if errors.Is(err, app.ErrStaleAccountNetwork) {
			cancel()
		}
		return conn, err
	}
	b.peer.Start()
	startCtx, startCancel := context.WithTimeout(ctx, 25*time.Second)
	frame, e := b.peer.Call(startCtx, providerbrowser.Action{Kind: "start", URL: startURL, Timezone: route.Timezone, Locale: geoctx.LocaleForCountry(route.Country), Width: req.Width, Height: req.Height})
	startCancel()
	if e != nil || !frame.Isolated {
		writeJSON(w, 503, map[string]string{"error": "browser_start_failed", "detail": "The isolated browser could not start. No direct connection is permitted."})
		return
	}
	if !stopRequest() && r.Context().Err() != nil {
		return
	}
	if _, e = s.Auth.Authenticate(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); e != nil {
		writeJSON(w, 401, errorBody())
		return
	}
	s.BrowserMu.Lock()
	s.ProviderBrowsers[id] = b
	s.BrowserMu.Unlock()
	published = true
	go s.watchProviderBrowser(ctx, id, b)
	writeJSON(w, 201, map[string]any{"id": id, "provider": req.Provider, "country": route.Country, "exit_ip": route.IP, "proxy_id": route.ProxyID, "expires_at": b.expires, "frame": frame})
}
func (s *Server) watchProviderBrowser(ctx context.Context, id string, b *providerBrowserSession) {
	defer func() { s.BrowserMu.Lock(); delete(s.ProviderBrowsers, id); s.BrowserMu.Unlock(); b.close() }()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.peer.Done():
			return
		case <-ticker.C:
			b.mu.Lock()
			idle := time.Since(b.lastUsed) > 2*time.Minute
			b.mu.Unlock()
			if idle {
				return
			}
			checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			var authorized bool
			e := s.DB.QueryRowContext(checkCtx, `SELECT EXISTS(SELECT 1 FROM admin_sessions x JOIN admin_users u ON u.id=x.user_id WHERE x.id=$1 AND x.expires_at>now() AND u.enabled AND u.role='admin')`, b.login).Scan(&authorized)
			if e == nil && !authorized {
				e = errors.New("browser owner session ended")
			}
			if e == nil {
				e = b.route.Check(checkCtx)
			}
			if e == nil && b.authorization != "" {
				var active bool
				e = s.DB.QueryRowContext(checkCtx, `SELECT EXISTS(SELECT 1 FROM provider_authorizations WHERE id=$1 AND owner_id=$2 AND state IN ('PENDING','READY','VERIFIED') AND expires_at>now())`, b.authorization, b.owner).Scan(&active)
				if e == nil && !active {
					e = errors.New("authorization ended")
				}
			}
			if e == nil && b.account != "" {
				var provider string
				e = s.DB.QueryRowContext(checkCtx, `SELECT provider FROM accounts WHERE id=$1`, b.account).Scan(&provider)
				if provider != b.provider {
					e = errors.New("account changed")
				}
			}
			cancel()
			if e != nil {
				return
			}
		}
	}
}
func (s *Server) providerBrowserAction(w http.ResponseWriter, r *http.Request) {
	b := s.browserSession(w, r)
	if b == nil {
		return
	}
	var a providerbrowser.Action
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	dec.DisallowUnknownFields()
	if dec.Decode(&a) != nil || dec.Decode(new(any)) != io.EOF || a.Kind == "start" || a.Timezone != "" || a.Locale != "" {
		writeJSON(w, 400, errorBody())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 22*time.Second)
	defer cancel()
	frame, e := b.peer.Call(ctx, a)
	if e != nil {
		writeJSON(w, 409, map[string]string{"error": "browser_action_failed", "detail": "The browser could not complete this action. Check the session status before retrying."})
		return
	}
	writeJSON(w, 200, frame)
}
func (s *Server) stopProviderBrowser(w http.ResponseWriter, r *http.Request) {
	if !authorizationAdmin(w, r) {
		return
	}
	p, _ := principal(r.Context())
	s.BrowserMu.Lock()
	b := s.ProviderBrowsers[r.PathValue("id")]
	if b != nil && b.owner == p.UserID && b.login == p.SessionID {
		delete(s.ProviderBrowsers, r.PathValue("id"))
	} else {
		b = nil
	}
	s.BrowserMu.Unlock()
	if b != nil {
		b.close()
	}
	w.WriteHeader(204)
}
