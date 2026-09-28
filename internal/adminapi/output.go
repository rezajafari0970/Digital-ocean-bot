package adminapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/export"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/http"
	"strings"
	"sync"
	"time"
)

type outputInbound struct {
	ID             int64           `json:"id"`
	Remark         string          `json:"remark"`
	Protocol       string          `json:"protocol"`
	Port           int             `json:"port"`
	Enable         bool            `json:"enable"`
	Settings       json.RawMessage `json:"settings"`
	StreamSettings json.RawMessage `json:"streamSettings"`
}

func outputMap(raw json.RawMessage) (map[string]any, error) {
	var m map[string]any
	e := json.Unmarshal(raw, &m)
	return m, e
}

func (s *Server) collectPanelOutput(parent context.Context, p readyworker.Panel) string {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	var acc, host, base, user, pref string
	if s.DB.QueryRowContext(ctx, `SELECT pi.account_id::text,d.host,pi.base_url,x.username,x.password_secret_ref FROM panel_instances pi JOIN deployments d ON d.droplet_id=pi.droplet_id JOIN xui_panel_deployments x ON x.droplet_id=pi.droplet_id AND x.generation=d.postinstall_generation WHERE pi.id=$1 AND pi.enabled=true`, p.ID).Scan(&acc, &host, &base, &user, &pref) != nil {
		return ""
	}
	keys := map[int64]string{}
	rows, e := s.DB.QueryContext(ctx, `SELECT remote_id,public_key FROM inbound_export_metadata WHERE panel_id=$1`, p.ID)
	if e == nil {
		for rows.Next() {
			var id int64
			var key string
			if rows.Scan(&id, &key) == nil {
				keys[id] = key
			}
		}
		rows.Close()
	}
	password, e := s.Container.Secrets.Get(ctx, acc, pref)
	if e != nil {
		return ""
	}
	client, e := sanaei.NewAPIClient(base, sanaei.Credentials{Username: user, Password: string(password)}, nil)
	for i := range password {
		password[i] = 0
	}
	if e != nil {
		return ""
	}
	client.HTTP.Timeout = 7 * time.Second
	if client.Login(ctx) != nil {
		return ""
	}
	raws, e := sanaei.ReadRawInboundList(ctx, sanaei.DirectSessionExecutor{Client: client})
	if e != nil {
		return ""
	}
	var out strings.Builder
	for _, raw := range raws {
		var in outputInbound
		if json.Unmarshal(raw, &in) != nil || !in.Enable || in.Protocol != "vless" {
			continue
		}
		publicKey := keys[in.ID]
		if publicKey == "" {
			continue
		}
		settings, e := outputMap(in.Settings)
		if e != nil {
			continue
		}
		clients, _ := settings["clients"].([]any)
		if len(clients) == 0 {
			continue
		}
		stream, e := outputMap(in.StreamSettings)
		if e != nil || fmt.Sprint(stream["network"]) != "tcp" || fmt.Sprint(stream["security"]) != "reality" {
			continue
		}
		rb, _ := json.Marshal(stream["realitySettings"])
		reality, e := outputMap(rb)
		if e != nil {
			continue
		}
		var names, shorts []string
		b, _ := json.Marshal(reality["serverNames"])
		_ = json.Unmarshal(b, &names)
		b, _ = json.Marshal(reality["shortIds"])
		_ = json.Unmarshal(b, &shorts)
		if len(names) == 0 || len(shorts) == 0 {
			continue
		}
		for _, v := range clients {
			b, _ := json.Marshal(v)
			var cl map[string]any
			if json.Unmarshal(b, &cl) != nil {
				continue
			}
			id := fmt.Sprint(cl["id"])
			if id == "" {
				continue
			}
			remark := in.Remark
			if email := fmt.Sprint(cl["email"]); email != "" {
				remark += "-" + email
			}
			link, e := export.VLESSRealityURI(export.VLESSReality{UUID: id, Host: host, Port: in.Port, SNI: names[0], PublicKey: publicKey, ShortID: shorts[0], Fingerprint: "chrome", Flow: fmt.Sprint(cl["flow"]), Remark: remark})
			if e == nil {
				out.WriteString(link)
				out.WriteByte('\n')
			}
		}
	}
	return out.String()
}

func (s *Server) outputConfigs(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	panels, e := (readyworker.SQLSource{DB: s.DB}).EligibleReadyPanels(ctx)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	results := make([]string, len(panels))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, p := range panels {
		wg.Add(1)
		go func(i int, p readyworker.Panel) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			results[i] = s.collectPanelOutput(ctx, p)
		}(i, p)
	}
	wg.Wait()
	var out strings.Builder
	for _, v := range results {
		out.WriteString(v)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	_, _ = w.Write([]byte(out.String()))
}
