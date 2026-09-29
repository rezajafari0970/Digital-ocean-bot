package adminapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/export"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"log"
	"net/http"
	"strings"
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

type outputCacheEntry struct {
	Value string
	At    time.Time
}

func outputMap(raw json.RawMessage) (map[string]any, error) {
	var m map[string]any
	e := json.Unmarshal(raw, &m)
	return m, e
}

func (s *Server) cachedPanelOutput(panelID string, maxAge time.Duration) string {
	s.OutputCacheMu.RLock()
	e, ok := s.OutputCache[panelID]
	s.OutputCacheMu.RUnlock()
	if !ok || e.Value == "" || time.Since(e.At) > maxAge {
		return ""
	}
	return e.Value
}
func (s *Server) storePanelOutput(panelID, value string) {
	if value == "" {
		return
	}
	s.OutputCacheMu.Lock()
	if s.OutputCache == nil {
		s.OutputCache = map[string]outputCacheEntry{}
	}
	s.OutputCache[panelID] = outputCacheEntry{Value: value, At: time.Now()}
	s.OutputCacheMu.Unlock()
}

func (s *Server) refreshPanelOutput(parent context.Context, p readyworker.Panel) string {

	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()

	var runtime *sanaei.PanelRuntime
	var err error
	if s.OutputRuntimes != nil {
		runtime, err = s.OutputRuntimes.Acquire(ctx, p.ID)
	} else {
		runtime, err = (sanaei.RuntimeFactory{DB: s.DB, Secrets: s.Container.Secrets, Timeout: 5 * time.Second}).Open(ctx, p.ID)
	}
	if err != nil {
		log.Printf("output refresh panel=%s acquire_error=%v", p.ID, err)
		return s.cachedPanelOutput(p.ID, 24*time.Hour)
	}
	out := s.collectRuntimeOutput(ctx, p, runtime)
	if out != "" {
		s.persistOutputSnapshot(ctx, p.ID, out)
		return ""
	}
	if out == "" {
		log.Printf("output refresh panel=%s empty_output", p.ID)
	}
	return s.cachedPanelOutput(p.ID, 24*time.Hour)
}

func (s *Server) refreshPanelOutputAsync(p readyworker.Panel) {
	s.OutputRefreshMu.Lock()
	if s.OutputRefreshing == nil {
		s.OutputRefreshing = map[string]bool{}
	}
	if s.OutputRefreshing[p.ID] {
		s.OutputRefreshMu.Unlock()
		return
	}
	s.OutputRefreshing[p.ID] = true
	s.OutputRefreshMu.Unlock()
	go func() {
		defer func() { s.OutputRefreshMu.Lock(); delete(s.OutputRefreshing, p.ID); s.OutputRefreshMu.Unlock() }()
		parent := s.OutputContext
		if parent == nil {
			parent = context.Background()
		}
		ctx, cancel := context.WithTimeout(parent, 90*time.Second)
		defer cancel()
		_ = s.refreshPanelOutput(ctx, p)
	}()
}

func (s *Server) collectPanelOutput(_ context.Context, p readyworker.Panel) string {
	cached := s.cachedPanelOutput(p.ID, 24*time.Hour)
	s.refreshPanelOutputAsync(p)
	return cached
}

func (s *Server) WarmOutputCache(ctx context.Context) {
	s.OutputPauseMu.RLock()
	defer s.OutputPauseMu.RUnlock()
	panels, err := (readyworker.SQLSource{DB: s.DB}).EligibleReadyPanels(ctx)
	if err != nil {
		return
	}
	for _, p := range panels {
		if ctx.Err() != nil {
			return
		}
		_ = s.refreshPanelOutput(ctx, p)
	}
}

func (s *Server) collectRuntimeOutput(ctx context.Context, p readyworker.Panel, runtime *sanaei.PanelRuntime) string {
	if runtime == nil || runtime.Session == nil {
		return ""
	}
	var host string
	if s.DB.QueryRowContext(ctx,
		"SELECT d.host FROM panel_instances pi JOIN deployments d ON d.droplet_id=pi.droplet_id WHERE pi.id=$1 AND pi.enabled=true",
		p.ID,
	).Scan(&host) != nil {
		return ""
	}
	keys := map[int64]string{}
	rows, err := s.DB.QueryContext(ctx,
		"SELECT remote_id,public_key FROM inbound_export_metadata WHERE panel_id=$1",
		p.ID,
	)
	if err == nil {
		for rows.Next() {
			var id int64
			var key string
			if rows.Scan(&id, &key) == nil {
				keys[id] = key
			}
		}
		rows.Close()
	}
	raws, err := runtime.Session.Snapshot(ctx)
	if err != nil {
		log.Printf("output collect panel=%s host=%s snapshot_error=%v", p.ID, host, err)
		return ""
	}
	var out strings.Builder
	rawCount, invalidJSON, disabled, nonVLESS, missingKey, noClients, notReality, noNames, noShorts, exportErrors, generated := len(raws), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
	for _, raw := range raws {
		var in outputInbound
		if json.Unmarshal(raw, &in) != nil {
			invalidJSON++
			continue
		}
		if !in.Enable {
			disabled++
			continue
		}
		if in.Protocol != "vless" {
			nonVLESS++
			continue
		}
		publicKey := keys[in.ID]
		if publicKey == "" {
			missingKey++
			continue
		}
		settings, e := outputMap(in.Settings)
		if e != nil {
			continue
		}
		clients, _ := settings["clients"].([]any)
		if len(clients) == 0 {
			noClients++
			continue
		}
		stream, e := outputMap(in.StreamSettings)
		if e != nil || fmt.Sprint(stream["network"]) != "tcp" || fmt.Sprint(stream["security"]) != "reality" {
			notReality++
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
		if len(names) == 0 {
			noNames++
			continue
		}
		if len(shorts) == 0 {
			noShorts++
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
				generated++
			} else {
				exportErrors++
			}
		}
	}
	log.Printf("output collect panel=%s host=%s raw=%d invalid_json=%d disabled=%d non_vless=%d missing_key=%d no_clients=%d not_reality=%d no_names=%d no_shorts=%d export_errors=%d generated=%d", p.ID, host, rawCount, invalidJSON, disabled, nonVLESS, missingKey, noClients, notReality, noNames, noShorts, exportErrors, generated)
	return out.String()
}

func (s *Server) outputConfigs(w http.ResponseWriter, r *http.Request) {
	s.outputSnapshotResponse(w, r)
}
