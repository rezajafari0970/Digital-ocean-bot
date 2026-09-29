package adminapi

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"sort"
	"strings"
)

type previewAccount struct {
	Provider string `json:"provider"`
	Token    string `json:"token"`
	ProxyID  string `json:"proxy_id"`
}
type previewCredential struct{ token []byte }

func (m previewCredential) Get(context.Context) ([]byte, error) {
	return append([]byte(nil), m.token...), nil
}

func (s *Server) accountPreview(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x previewAccount
	if json.NewDecoder(r.Body).Decode(&x) != nil || strings.TrimSpace(x.Token) == "" {
		writeJSON(w, 400, map[string]string{"error": "token_required"})
		return
	}
	if x.Provider == "" {
		x.Provider = "digitalocean"
	}
	if s.Container.Providers == nil || !s.Container.Providers.Has(x.Provider) {
		writeJSON(w, 400, map[string]string{"error": "provider_not_supported"})
		return
	}
	var client *http.Client
	var closeFn func()
	if x.ProxyID != "" {
		var px network.Proxy
		var user, ref string
		if err := s.DB.QueryRowContext(r.Context(), `SELECT id::text,name,type,host,port,COALESCE(username,''),COALESCE(secret_ref,''),status FROM proxies WHERE id=$1`, x.ProxyID).Scan(&px.ID, &px.Name, &px.Type, &px.Host, &px.Port, &user, &ref, &px.Status); err != nil || px.Status != network.StatusHealthy {
			writeJSON(w, 409, map[string]string{"error": "proxy_not_healthy"})
			return
		}
		pass := []byte(nil)
		if ref != "" {
			var err error
			pass, err = s.Container.Secrets.GetProxy(r.Context(), px.ID, ref)
			if err != nil {
				writeJSON(w, 500, errorBody())
				return
			}
			defer zeroBytes(pass)
		}
		g, err := network.NewProxyGateway("preview", px, network.ProxyCredentials{Username: user, Password: string(pass)})
		if err != nil {
			writeJSON(w, 409, map[string]string{"error": "proxy_not_ready"})
			return
		}
		client = g.Client
		closeFn = g.CloseIdleConnections
	} else {
		b, err := network.NewIsolatedDirectClient("preview")
		if err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		client = b.Client
		closeFn = b.CloseIdleConnections
	}
	defer closeFn()
	driver, err := s.Container.Providers.Open(r.Context(), x.Provider, providers.OpenRequest{AccountID: "preview", HTTPClient: client, Credentials: previewCredential{[]byte(x.Token)}})
	if err != nil {
		writeJSON(w, 422, map[string]string{"error": "provider_validation_failed", "detail": err.Error()})
		return
	}
	ar, aok := driver.(providers.AccountReader)
	cr, cok := driver.(providers.CatalogReader)
	if !aok || !cok {
		writeJSON(w, 422, map[string]string{"error": "provider_capability_missing"})
		return
	}
	account, err := ar.Account(r.Context())
	if err != nil {
		writeProviderPreviewError(w, err)
		return
	}
	capacity, capErr := ar.Capacity(r.Context())
	if capErr != nil {
		writeProviderPreviewError(w, capErr)
		return
	}
	catalog, err := cr.Catalog(r.Context())
	if err != nil {
		writeProviderPreviewError(w, err)
		return
	}
	regions := catalog.Regions
	sort.SliceStable(regions, func(i, j int) bool { return regions[i].Name < regions[j].Name })
	images := catalog.Images
	sort.SliceStable(images, func(i, j int) bool {
		ui := images[i].Family == "ubuntu"
		uj := images[j].Family == "ubuntu"
		if ui != uj {
			return ui
		}
		return images[i].Version > images[j].Version
	})
	proxies := []map[string]any{{"id": "", "name": "No proxy — Direct server IP", "status": "direct"}}
	rows, _ := s.DB.QueryContext(r.Context(), `SELECT id::text,name,status,COALESCE(country,'') FROM proxies WHERE status='healthy' ORDER BY name`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var id, n, st, c string
			if rows.Scan(&id, &n, &st, &c) == nil {
				proxies = append(proxies, map[string]any{"id": id, "name": n, "status": st, "country": c})
			}
		}
	}
	writeJSON(w, 200, map[string]any{"provider": driver.Name(), "account": account, "server_limit": capacity.ComputeLimit, "provider_servers": capacity.ComputeInUse, "regions": regions, "plans": catalog.Plans, "sizes": catalog.Plans, "images": images, "proxies": proxies, "defaults": map[string]any{"interval_seconds": 300, "batch_size": 1, "max_concurrent": 1}})
}
func writeProviderPreviewError(w http.ResponseWriter, err error) {
	switch providers.Class(err) {
	case providers.ErrorAuthentication:
		writeJSON(w, 401, map[string]string{"error": "provider_credential_invalid", "detail": "Provider rejected these credentials."})
	case providers.ErrorPermissionDenied, providers.ErrorAccountLocked:
		writeJSON(w, 403, map[string]string{"error": "provider_permission_denied", "detail": err.Error()})
	case providers.ErrorRateLimited:
		writeJSON(w, 429, map[string]string{"error": "provider_rate_limited", "detail": "Provider rate limit reached. Try again later."})
	case providers.ErrorTransport, providers.ErrorUnavailable:
		writeJSON(w, 503, map[string]string{"error": "provider_transport_failed", "detail": "Could not reach provider through the selected connection."})
	default:
		writeJSON(w, 422, map[string]string{"error": "provider_validation_failed", "detail": err.Error()})
	}
}
