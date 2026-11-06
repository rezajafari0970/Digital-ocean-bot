package adminapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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
	"github.com/rezajafari0970/Digital-ocean-bot/internal/nativebrowser"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providerbrowser"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ovh"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxyselection"
	"golang.org/x/net/websocket"
)

// This separate transport leaves rendering, input, TLS and cookies in the user's
// real browser. Only temporary client credentials leave the control plane.
type nativeBrowserSession struct {
	owner, login, provider, account, authorization, website string
	credential                                              [16]byte
	route                                                   *app.OnboardingRoute
	ctx                                                     context.Context
	cancel                                                  context.CancelFunc
	expires                                                 time.Time
	mu                                                      sync.Mutex
	closed                                                  bool
	reason                                                  string
	connections                                             int
	opened, transferred                                     int64
	lastUsed                                                time.Time
}

func (b *nativeBrowserSession) close(reason string) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.reason = reason
	b.mu.Unlock()
	b.cancel()
	if b.route != nil {
		b.route.Close()
	}
}
func (b *nativeBrowserSession) state(id string) map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	state := "active"
	if b.closed {
		state = "closed"
	}
	return map[string]any{"id": id, "state": state, "reason": b.reason, "provider": b.provider, "website": b.website, "country": b.route.Country, "exit_ip": b.route.IP, "proxy_id": b.route.ProxyID, "expires_at": b.expires, "connections": b.connections, "successful_connections": b.opened, "bytes": b.transferred}
}
func (s *Server) closeNativeBrowsers(login string) {
	s.NativeMu.Lock()
	var list []*nativeBrowserSession
	for _, b := range s.NativeBrowsers {
		if b.login == login {
			list = append(list, b)
		}
	}
	s.NativeMu.Unlock()
	for _, b := range list {
		b.close("signed_out")
	}
}
func (s *Server) ownedNativeBrowser(w http.ResponseWriter, r *http.Request) *nativeBrowserSession {
	w.Header().Set("Cache-Control", "no-store")
	if !authorizationAdmin(w, r) {
		return nil
	}
	p, _ := principal(r.Context())
	s.NativeMu.Lock()
	b := s.NativeBrowsers[r.PathValue("id")]
	s.NativeMu.Unlock()
	if b == nil || b.owner != p.UserID || b.login != p.SessionID {
		writeJSON(w, 404, map[string]string{"error": "native_session_unavailable"})
		return nil
	}
	return b
}
func (s *Server) nativeBrowserStatus(w http.ResponseWriter, r *http.Request) {
	if b := s.ownedNativeBrowser(w, r); b != nil {
		writeJSON(w, 200, b.state(r.PathValue("id")))
	}
}
func (s *Server) stopNativeBrowser(w http.ResponseWriter, r *http.Request) {
	if b := s.ownedNativeBrowser(w, r); b != nil {
		b.close("ended_by_user")
		w.WriteHeader(204)
	}
}
func (s *Server) startNativeBrowser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
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
		ProxyCountries  []string `json:"proxy_countries"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || dec.Decode(new(any)) != io.EOF {
		writeJSON(w, 400, errorBody())
		return
	}
	if req.AccountID != "" {
		var provider string
		if s.DB == nil || s.DB.QueryRowContext(r.Context(), `SELECT provider FROM accounts WHERE id=$1 AND deletion_requested_at IS NULL`, req.AccountID).Scan(&provider) != nil || provider != req.Provider {
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
	countries, e := proxyselection.Priorities("", req.ProxyCountries)
	ids := normalizeProxyPool(req.ProxyID, req.ProxyIDs)
	if e != nil || req.ProxyID == "" || len(ids) > 3 {
		writeJSON(w, 400, map[string]string{"error": "browser_proxy_required", "detail": "Select a proxy and up to three country priorities."})
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
	s.NativeMu.Lock()
	if s.NativeBrowsers == nil {
		s.NativeBrowsers = map[string]*nativeBrowserSession{}
	}
	for id, b := range s.NativeBrowsers {
		b.mu.Lock()
		closed := b.closed
		b.mu.Unlock()
		if closed {
			delete(s.NativeBrowsers, id)
		}
	}
	if len(s.NativeBrowsers)+s.NativeStarting >= 2 {
		s.NativeMu.Unlock()
		writeJSON(w, 429, map[string]string{"error": "native_capacity_full", "detail": "End an existing native browser connection before creating another."})
		return
	}
	s.NativeStarting++
	s.NativeMu.Unlock()
	defer func() { s.NativeMu.Lock(); s.NativeStarting--; s.NativeMu.Unlock() }()
	var token [24]byte
	var key [16]byte
	if _, e = rand.Read(token[:]); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if _, e = rand.Read(key[:]); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	key[6] = key[6]&15 | 64
	key[8] = key[8]&63 | 128
	id := hex.EncodeToString(token[:])
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	b := &nativeBrowserSession{owner: p.UserID, login: p.SessionID, provider: req.Provider, account: req.AccountID, authorization: req.AuthorizationID, website: startURL, credential: key, ctx: ctx, cancel: cancel, expires: time.Now().Add(15 * time.Minute), lastUsed: time.Now()}
	published := false
	defer func() {
		if !published {
			b.close("startup_failed")
		}
	}()
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
	if !stopRequest() && r.Context().Err() != nil {
		return
	}
	if _, e = s.Auth.Authenticate(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); e != nil {
		writeJSON(w, 401, errorBody())
		return
	}
	s.NativeMu.Lock()
	s.NativeBrowsers[id] = b
	s.NativeMu.Unlock()
	published = true
	go s.watchNativeBrowser(b)
	value := b.state(id)
	raw := hex.EncodeToString(key[:])
	value["credential"] = raw[:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:]
	value["path"] = "/api/v1/provider-browser/native-stream/" + id
	writeJSON(w, 201, value)
}
func (s *Server) watchNativeBrowser(b *nativeBrowserSession) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-b.ctx.Done():
			b.close("expired")
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(b.ctx, 8*time.Second)
			var valid bool
			err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_sessions x JOIN admin_users u ON u.id=x.user_id WHERE x.id=$1 AND x.expires_at>now() AND u.enabled AND u.role='admin')`, b.login).Scan(&valid)
			reason := "authorization_unavailable"
			if err == nil && !valid {
				err = errors.New("signed out")
				reason = "signed_out"
			}
			if err == nil && b.account != "" {
				err = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1 AND provider=$2 AND deletion_requested_at IS NULL)`, b.account, b.provider).Scan(&valid)
				if err == nil && !valid {
					err = errors.New("account ended")
				}
				reason = "account_changed"
			}
			if err == nil && b.authorization != "" {
				err = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM provider_authorizations WHERE id=$1 AND owner_id=$2 AND state IN ('PENDING','READY','VERIFIED') AND expires_at>now())`, b.authorization, b.owner).Scan(&valid)
				if err == nil && !valid {
					err = errors.New("authorization ended")
				}
				reason = "authorization_ended"
			}
			if err == nil {
				err = b.route.Check(ctx)
				reason = "proxy_identity_unverified"
			}
			cancel()
			if err != nil {
				b.close(reason)
				return
			}
		}
	}
}

// This route authenticates a bounded VLESS handshake, not a panel cookie/token.
// A random path alone grants no egress. Browser-origin WebSockets are rejected.
func (s *Server) nativeBrowserStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	if len(id) != 48 || r.Header.Get("Origin") != "" {
		http.NotFound(w, r)
		return
	}
	if _, err := hex.DecodeString(id); err != nil {
		http.NotFound(w, r)
		return
	}
	s.NativeMu.Lock()
	b := s.NativeBrowsers[id]
	if b == nil || s.NativeConnections >= 64 {
		s.NativeMu.Unlock()
		http.NotFound(w, r)
		return
	}
	b.mu.Lock()
	if b.closed || !b.expires.After(time.Now()) || b.connections >= 32 {
		b.mu.Unlock()
		s.NativeMu.Unlock()
		http.NotFound(w, r)
		return
	}
	b.connections++
	s.NativeConnections++
	b.mu.Unlock()
	s.NativeMu.Unlock()
	defer func() {
		s.NativeMu.Lock()
		s.NativeConnections--
		s.NativeMu.Unlock()
		b.mu.Lock()
		b.connections--
		b.mu.Unlock()
	}()
	server := websocket.Server{Handshake: func(_ *websocket.Config, request *http.Request) error {
		if request.Header.Get("Origin") != "" {
			return errors.New("browser origin not accepted")
		}
		return nil
	}, Handler: func(conn *websocket.Conn) {
		conn.PayloadType = websocket.BinaryFrame
		conn.MaxPayloadBytes = 1 << 20
		dial := func(ctx context.Context, address string) (net.Conn, error) {
			upstream, err := b.route.Dial(ctx, address)
			if errors.Is(err, app.ErrStaleAccountNetwork) {
				b.close("proxy_identity_unverified")
			}
			return upstream, err
		}
		_ = nativebrowser.Relay(b.ctx, conn, b.credential, dial, func() { b.mu.Lock(); b.opened++; b.lastUsed = time.Now(); b.mu.Unlock() }, func(n int64) { b.mu.Lock(); b.transferred += n; b.lastUsed = time.Now(); b.mu.Unlock() })
	}}
	server.ServeHTTP(w, nativeGatewayWebSocketRequest(r))
}

// The loopback HTTPS gateway can remove hop-by-hop Upgrade headers while keeping
// the end-to-end Sec-WebSocket handshake. Nginx still tunnels a 101 response when
// the original client requested Upgrade. Reconstruct only that bounded handshake;
// the temporary path and VLESS UUID remain required before any upstream egress.
func nativeGatewayWebSocketRequest(r *http.Request) *http.Request {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	key, keyErr := base64.StdEncoding.DecodeString(r.Header.Get("Sec-WebSocket-Key"))
	upgrade := r.Header.Get("Upgrade")
	if err != nil || ip == nil || !ip.IsLoopback() || r.ProtoMajor != 1 || r.Header.Get("Origin") != "" || r.Header.Get("Sec-WebSocket-Version") != "13" || keyErr != nil || len(key) != 16 || (upgrade != "" && !strings.EqualFold(upgrade, "websocket")) {
		return r
	}
	copy := r.Clone(r.Context())
	copy.Header.Set("Upgrade", "websocket")
	copy.Header.Set("Connection", "Upgrade")
	return copy
}
