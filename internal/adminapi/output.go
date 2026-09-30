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

type outputRecord struct {
	URI          string
	VisibleUntil *time.Time
}

func outputVisibleUntil(raw any) *time.Time {
	expiryMillis, ok := raw.(float64)
	if !ok || expiryMillis <= 0 {
		return nil
	}
	t := time.UnixMilli(int64(expiryMillis)).UTC().Add(-10 * time.Second)
	return &t
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
	records, ok := s.collectRuntimeOutput(ctx, p, runtime)
	if ok {
		s.persistOutputSnapshot(ctx, p.ID, records)
		if len(records) == 0 {
			log.Printf("output refresh panel=%s empty_output", p.ID)
		}
		return ""
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
	// Snapshots are a materialized view of currently eligible Sanaei panels only.
	// Purge rows for panels that have become disabled, locked, non-terminal, or retired.
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM output_config_snapshots o WHERE NOT EXISTS (
		SELECT 1 FROM panel_instances p
		JOIN droplets dr ON dr.id=p.droplet_id
		JOIN accounts a ON a.id=dr.account_id
		JOIN deployments d ON d.droplet_id=dr.id
		WHERE p.id=o.panel_id AND p.enabled=true AND d.state='PANEL_COMPLETE'
		AND a.provider_state<>'LOCKED' AND dr.state IN ('READY','EXPIRING','RETIRING')
		AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 seconds')
	)`)
	for _, p := range panels {
		if ctx.Err() != nil {
			return
		}
		_ = s.refreshPanelOutput(ctx, p)
	}
}

func (s *Server) collectRuntimeOutput(ctx context.Context, p readyworker.Panel, runtime *sanaei.PanelRuntime) ([]outputRecord, bool) {
	if runtime == nil || runtime.Session == nil {
		return nil, false
	}
	var host string
	if s.DB.QueryRowContext(ctx,
		"SELECT d.host FROM panel_instances pi JOIN deployments d ON d.droplet_id=pi.droplet_id WHERE pi.id=$1 AND pi.enabled=true",
		p.ID,
	).Scan(&host) != nil {
		return nil, false
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
	// Output is a near-real-time materialized view. Client mutations happen in the
	// worker process, so the API process must not reuse its in-memory Sanaei snapshot.
	runtime.Session.Invalidate()
	raws, err := runtime.Session.Snapshot(ctx)
	if err != nil {
		log.Printf("output collect panel=%s host=%s snapshot_error=%v", p.ID, host, err)
		return nil, false
	}
	var out strings.Builder
	records := make([]outputRecord, 0)
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
		stream, e := outputMap(in.StreamSettings)
		if e != nil || fmt.Sprint(stream["network"]) != "tcp" || fmt.Sprint(stream["security"]) != "reality" {
			notReality++
			continue
		}
		structSettings := make(map[string]any, len(settings))
		for k, v := range settings {
			structSettings[k] = v
		}
		structSettings["clients"] = []any{}
		structPayload := map[string]any{"enable": in.Enable, "remark": in.Remark, "port": in.Port, "protocol": in.Protocol, "settings": structSettings, "streamSettings": stream}
		if sb, err := json.Marshal(structPayload); err == nil {
			_, _ = s.DB.ExecContext(ctx, `INSERT INTO inbound_structural_snapshots(panel_id,remote_id,port,payload,updated_at) VALUES($1,$2,$3,$4,now()) ON CONFLICT(panel_id,remote_id) DO UPDATE SET port=excluded.port,payload=excluded.payload,updated_at=now()`, p.ID, in.ID, in.Port, sb)
		}
		if len(clients) == 0 {
			noClients++
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
			if enabled, ok := cl["enable"].(bool); ok && !enabled {
				continue
			}
			remark := in.Remark
			if email := fmt.Sprint(cl["email"]); email != "" {
				remark += "-" + email
			}
			link, e := export.VLESSRealityURI(export.VLESSReality{UUID: id, Host: host, Port: in.Port, SNI: names[0], PublicKey: publicKey, ShortID: shorts[0], Fingerprint: "chrome", Flow: fmt.Sprint(cl["flow"]), Remark: remark})
			if e == nil {
				visibleUntil := outputVisibleUntil(cl["expiryTime"])
				records = append(records, outputRecord{URI: link, VisibleUntil: visibleUntil})
				out.WriteString(link)
				out.WriteByte('\n')
				generated++
			} else {
				exportErrors++
			}
		}
	}
	log.Printf("output collect panel=%s host=%s raw=%d invalid_json=%d disabled=%d non_vless=%d missing_key=%d no_clients=%d not_reality=%d no_names=%d no_shorts=%d export_errors=%d generated=%d", p.ID, host, rawCount, invalidJSON, disabled, nonVLESS, missingKey, noClients, notReality, noNames, noShorts, exportErrors, generated)
	s.storePanelOutput(p.ID, out.String())
	return records, true
}

func (s *Server) outputConfigs(w http.ResponseWriter, r *http.Request) {
	s.outputSnapshotResponse(w, r)
}
