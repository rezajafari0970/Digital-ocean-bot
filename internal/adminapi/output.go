package adminapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/export"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"log"
	"math/rand"
	"net/http"
	"strconv"
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

	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
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
		s.storePanelOutput(p.ID, out)
		s.persistOutputSnapshot(ctx, p.ID, out)
		return out
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
		ctx, cancel := context.WithTimeout(parent, 6*time.Second)
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
	panels, err := (readyworker.SQLSource{DB: s.DB}).EligibleReadyPanels(ctx)
	if err != nil {
		return
	}
	for _, p := range panels {
		s.refreshPanelOutputAsync(p)
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
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	source := readyworker.SQLSource{DB: s.DB}
	panels, e := source.EligibleReadyPanels(ctx)
	if raw := r.URL.Query().Get("expires_within_minutes"); raw != "" {
		minutes, err := strconv.Atoi(raw)
		if err != nil || minutes < 1 || minutes > 1440 {
			http.Error(w, "invalid expiry window", http.StatusBadRequest)
			return
		}
		panels, e = source.EligibleExpiringPanels(ctx, minutes)
	}
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	active := make(map[string]bool, len(panels))
	for _, p := range panels {
		active[p.ID] = true
	}
	s.OutputCacheMu.Lock()
	for id := range s.OutputCache {
		if !active[id] {
			delete(s.OutputCache, id)
		}
	}
	s.OutputCacheMu.Unlock()
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
	// Randomized round-robin across servers: configs from one server are not
	// emitted as one large consecutive block.
	rand.Shuffle(len(results), func(i, j int) { results[i], results[j] = results[j], results[i] })
	queues := make([][]string, 0, len(results))
	for _, v := range results {
		lines := strings.FieldsFunc(v, func(r rune) bool { return r == '\n' || r == '\r' })
		if len(lines) > 0 {
			rand.Shuffle(len(lines), func(i, j int) { lines[i], lines[j] = lines[j], lines[i] })
			queues = append(queues, lines)
		}
	}
	var out strings.Builder
	for remaining := true; remaining; {
		remaining = false
		rand.Shuffle(len(queues), func(i, j int) { queues[i], queues[j] = queues[j], queues[i] })
		for i := range queues {
			if len(queues[i]) == 0 {
				continue
			}
			remaining = true
			out.WriteString(queues[i][0])
			out.WriteByte('\n')
			queues[i] = queues[i][1:]
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	_, _ = w.Write([]byte(out.String()))
}
