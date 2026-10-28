package adminapi

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) previewClient(w http.ResponseWriter, r *http.Request, proxyID string, probe bool) (*http.Client, func(), string, bool) {
	previewExitIP := ""
	var client *http.Client
	var closeFn func()
	if proxyID != "" {
		var px network.Proxy
		var user, ref, adapter, secretVersion string
		if err := s.DB.QueryRowContext(r.Context(), `SELECT id::text,name,type,host,port,COALESCE(username,''),COALESCE(secret_ref,''),status,COALESCE(adapter,'generic') ,COALESCE((SELECT sec.xmin::text||':'||sec.updated_at::text FROM secrets sec WHERE sec.proxy_id=proxies.id AND sec.id=proxies.secret_ref),'') FROM proxies WHERE id=$1`, proxyID).Scan(&px.ID, &px.Name, &px.Type, &px.Host, &px.Port, &user, &ref, &px.Status, &adapter, &secretVersion); err != nil || px.Status != network.StatusHealthy {
			writeJSON(w, 409, map[string]string{"error": "proxy_not_healthy"})
			return nil, nil, "", false
		}
		pass := []byte(nil)
		if ref != "" {
			var err error
			pass, err = s.Container.Secrets.GetProxyAtVersion(r.Context(), px.ID, ref, secretVersion)
			if err != nil {
				writeJSON(w, 500, errorBody())
				return nil, nil, "", false
			}
			defer zeroBytes(pass)
		}
		cap := app.ProxyAdapterCapabilities(adapter)
		if cap.StickySession {
			user = app.ProxySessionUsername(adapter, user, "", "preview-vultr", false)
		}
		g, err := network.NewProxyGateway("preview", px, network.ProxyCredentials{Username: user, Password: string(pass)})
		if err != nil {
			writeJSON(w, 409, map[string]string{"error": "proxy_not_ready"})
			return nil, nil, "", false
		}
		client = g.Client
		closeFn = g.CloseIdleConnections
		if probe {
			probeCtx, probeCancel := context.WithTimeout(r.Context(), 6*time.Second)
			probeReq, _ := http.NewRequestWithContext(probeCtx, http.MethodGet, "https://api.ipify.org", nil)
			if probeResp, probeErr := client.Do(probeReq); probeErr == nil && probeResp != nil {
				b, _ := io.ReadAll(io.LimitReader(probeResp.Body, 128))
				probeResp.Body.Close()
				if probeResp.StatusCode >= 200 && probeResp.StatusCode < 300 {
					previewExitIP = strings.TrimSpace(string(b))
				}
			}
			probeCancel()
		}
	} else {
		b, err := network.NewIsolatedDirectClient("preview")
		if err != nil {
			writeJSON(w, 500, errorBody())
			return nil, nil, "", false
		}
		client = b.Client
		closeFn = b.CloseIdleConnections
	}
	return client, closeFn, previewExitIP, true
}
