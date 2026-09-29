package adminapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/http"
	"time"
)

type cleanupInbound struct {
	ID                                 int    `json:"id"`
	Remark                             string `json:"remark"`
	Listen                             string `json:"listen"`
	Port                               int    `json:"port"`
	Protocol                           string `json:"protocol"`
	Enable                             bool   `json:"enable"`
	ExpiryTime                         int64  `json:"expiryTime"`
	Total                              int64  `json:"total"`
	Settings, StreamSettings, Sniffing json.RawMessage
}
type cleanupResult struct {
	PanelID string `json:"panel_id"`
	Deleted int    `json:"deleted"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) deleteAllCapacityClients(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, e := s.DB.ExecContext(ctx, "UPDATE global_config_policies SET enabled=false,updated_at=now() WHERE policy_key='reality'")
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	panels, e := (readyworker.SQLSource{DB: s.DB}).EligibleReadyPanels(ctx)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	results := make([]cleanupResult, 0, len(panels))
	ok, failed := 0, 0
	for _, p := range panels {
		rr := cleanupResult{PanelID: p.ID}
		c, cancel := context.WithTimeout(ctx, 3*time.Minute)
		rr.Deleted, e = s.clearPanelClients(c, p)
		cancel()
		if e != nil {
			rr.Error = e.Error()
			failed++
		} else {
			ok++
		}
		results = append(results, rr)
	}
	_, _ = s.DB.ExecContext(ctx, "DELETE FROM user_capacity_snapshots")
	writeJSON(w, 200, map[string]any{"succeeded": ok, "failed": failed, "policy_enabled": false, "results": results})
}
func (s *Server) clearPanelClients(ctx context.Context, p readyworker.Panel) (int, error) {
	rt, e := (sanaei.RuntimeFactory{DB: s.DB, Secrets: s.Container.Secrets, Timeout: 90 * time.Second}).Open(ctx, p.ID)
	if e != nil {
		return 0, e
	}
	raws, e := rt.Session.Snapshot(ctx)
	if e != nil {
		return 0, e
	}
	deleted := 0
	for _, raw := range raws {
		var in cleanupInbound
		if json.Unmarshal(raw, &in) != nil || !in.Enable || in.Protocol != "vless" || (in.Port != 443 && in.Port != 7231 && in.Port != 1212) {
			continue
		}
		var stream, settings map[string]any
		if json.Unmarshal(in.StreamSettings, &stream) != nil || fmt.Sprint(stream["network"]) != "tcp" || fmt.Sprint(stream["security"]) != "reality" {
			continue
		}
		if json.Unmarshal(in.Settings, &settings) != nil {
			continue
		}
		if a, ok := settings["clients"].([]any); ok {
			deleted += len(a)
		}
		settings["clients"] = []any{}
		var sniff any = map[string]any{"enabled": false}
		if len(in.Sniffing) > 0 {
			_ = json.Unmarshal(in.Sniffing, &sniff)
		}
		payload := map[string]any{"enable": in.Enable, "remark": in.Remark, "listen": in.Listen, "port": in.Port, "protocol": in.Protocol, "expiryTime": in.ExpiryTime, "total": in.Total, "settings": settings, "streamSettings": stream, "sniffing": sniff}
		if _, e = sanaei.UpdateInboundRaw(ctx, rt.Session.Exec, int64(in.ID), payload); e != nil {
			return deleted, e
		}
	}
	rt.Session.Invalidate()
	return deleted, nil
}
