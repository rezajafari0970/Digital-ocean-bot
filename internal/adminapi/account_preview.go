package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/accounts"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"net/http"
	"sort"
	"strings"
)

type previewAccount struct {
	Token   string `json:"token"`
	ProxyID string `json:"proxy_id"`
}
type memorySecret struct{ token []byte }

func (m memorySecret) Get(context.Context, string, string) ([]byte, error) {
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
	cell := accounts.NewCellManager().Register("preview")
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
	provider, err := digitalocean.NewClient(cell.Context, "preview-token", memorySecret{[]byte(x.Token)}, client)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	d, err := provider.Catalog(r.Context())
	if err != nil {
		var h digitalocean.HTTPError
		if errors.As(err, &h) {
			switch h.Status {
			case 401:
				writeJSON(w, 401, map[string]string{"error": "digitalocean_token_invalid", "detail": "DigitalOcean rejected this API token."})
			case 403:
				writeJSON(w, 403, map[string]string{"error": "digitalocean_permission_denied", "detail": "The token does not have the required DigitalOcean permissions."})
			case 429:
				writeJSON(w, 429, map[string]string{"error": "digitalocean_rate_limited", "detail": "DigitalOcean rate limit reached. Try again later."})
			default:
				writeJSON(w, 422, map[string]string{"error": "digitalocean_validation_failed", "detail": err.Error()})
			}
			return
		}
		if errors.Is(err, digitalocean.ErrProviderRequest) {
			writeJSON(w, 503, map[string]string{"error": "digitalocean_transport_failed", "detail": "Could not reach DigitalOcean through the selected connection."})
			return
		}
		writeJSON(w, 422, map[string]string{"error": "digitalocean_validation_failed", "detail": err.Error()})
		return
	}
	regions := d.Regions
	sort.SliceStable(regions, func(i, j int) bool {
		fi := regions[i].Slug == "fra1"
		fj := regions[j].Slug == "fra1"
		if fi != fj {
			return fi
		}
		return regions[i].Name < regions[j].Name
	})
	images := d.Images
	sort.SliceStable(images, func(i, j int) bool {
		ui := strings.EqualFold(images[i].Distribution, "Ubuntu")
		uj := strings.EqualFold(images[j].Distribution, "Ubuntu")
		if ui != uj {
			return ui
		}
		return images[i].Name > images[j].Name
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
	writeJSON(w, 200, map[string]any{"account": d.Account, "regions": regions, "sizes": d.Sizes, "images": images, "proxies": proxies, "defaults": map[string]any{"region": "fra1", "interval_seconds": 300, "batch_size": 1, "max_concurrent": 1}})
}
