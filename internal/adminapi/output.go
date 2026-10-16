package adminapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/export"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"log"
	"net/http"
	"sort"
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

const outputDropletStatePredicate = "dr.state IN ('READY','EXPIRING')"

type outputRecord struct {
	ClientID     string
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

func (s *Server) refreshPanelOutput(parent context.Context, p readyworker.Panel) string {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	acquired, err := sanaei.TryWithConfigLock(ctx, s.DB, p.ID, func(ctx context.Context) error {
		var busy bool
		if err := s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM reality_transport_apply_jobs WHERE panel_id=$1 AND state IN('RUNNING','BLOCKED'))", p.ID).Scan(&busy); err != nil {
			return err
		}
		if !busy {
			s.refreshPanelOutputLocked(ctx, p)
		}
		return nil
	})
	if err != nil {
		log.Printf("output refresh panel=%s lock_error=%v", p.ID, err)
	} else if !acquired {
		log.Printf("output refresh panel=%s skipped=config_lock_busy", p.ID)
	}
	return ""
}
func (s *Server) refreshPanelOutputLocked(parent context.Context, p readyworker.Panel) string {

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
		return ""
	}
	records, ok := s.collectRuntimeOutput(ctx, p, runtime)
	if ok {
		if err := s.persistOutputSnapshot(ctx, p.ID, records); err != nil {
			return ""
		}
		if len(records) == 0 {
			log.Printf("output refresh panel=%s empty_output", p.ID)
		}
		return ""
	}
	return ""
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
		AND a.provider_state='ACTIVE' AND `+outputDropletStatePredicate+`
		AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 seconds')
	)`)
	if len(panels) == 0 || ctx.Err() != nil {
		return
	}
	s.refreshOutputLive(ctx)
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
	raws, err := runtime.Session.FreshSnapshot(ctx)
	if err != nil {
		log.Printf("output collect panel=%s host=%s snapshot_error=%v", p.ID, host, err)
		return nil, false
	}
	pins := map[string]transportprofile.Config{}
	pinRows, pinErr := s.DB.QueryContext(ctx, "SELECT managed_key,config FROM reality_listener_profiles WHERE panel_id=$1", p.ID)
	if pinErr != nil {
		return nil, false
	}
	for pinRows.Next() {
		var key string
		var raw []byte
		var c transportprofile.Config
		if pinRows.Scan(&key, &raw) != nil || json.Unmarshal(raw, &c) != nil {
			pinRows.Close()
			return nil, false
		}
		pins[key] = c
	}
	pinErr = pinRows.Err()
	pinRows.Close()
	if pinErr != nil {
		return nil, false
	}
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
		if e != nil || !transportprofile.Supported(fmt.Sprint(stream["network"])) || fmt.Sprint(stream["security"]) != "reality" {
			notReality++
			continue
		}
		transport, err := transportprofile.FromStream(stream)
		if err != nil {
			exportErrors++
			continue
		}
		if pin, ok := pins[in.Remark]; ok {
			transport, err = transportprofile.ApplyPinnedClientOptions(transport, pin)
			if err != nil {
				exportErrors++
				continue
			}
		}
		decryption, _ := settings["decryption"].(string)
		encryption, err := transportprofile.ClientEncryption(decryption)
		if err != nil {
			exportErrors++
			continue
		}
		if advertised, ok := settings["encryption"].(string); ok && advertised != "" && advertised != encryption {
			exportErrors++
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
			flow, _ := cl["flow"].(string)
			flow, e = transportprofile.ObservedClientFlow(transport, flow)
			if e != nil {
				exportErrors++
				continue
			}
			link, e := export.VLESSRealityURI(export.VLESSReality{Transport: transport, UUID: id, Host: host, Port: in.Port, SNI: names[0], PublicKey: publicKey, ShortID: shorts[0], Fingerprint: "chrome", Encryption: encryption, Flow: flow, Remark: remark})
			if e == nil {
				visibleUntil := outputVisibleUntil(cl["expiryTime"])
				records = append(records, outputRecord{ClientID: id, URI: link, VisibleUntil: visibleUntil})
				generated++
			} else {
				exportErrors++
			}
		}
	}
	log.Printf("output collect panel=%s host=%s raw=%d invalid_json=%d disabled=%d non_vless=%d missing_key=%d no_clients=%d not_reality=%d no_names=%d no_shorts=%d export_errors=%d generated=%d", p.ID, host, rawCount, invalidJSON, disabled, nonVLESS, missingKey, noClients, notReality, noNames, noShorts, exportErrors, generated)
	return records, true
}

func (s *Server) outputConfigs(w http.ResponseWriter, r *http.Request) {
	s.outputSnapshotResponse(w, r)
}

func rotateOutputPanels(panels []readyworker.Panel, start int) []readyworker.Panel {
	if len(panels) == 0 {
		return nil
	}
	start %= len(panels)
	if start < 0 {
		start += len(panels)
	}
	out := make([]readyworker.Panel, 0, len(panels))
	out = append(out, panels[start:]...)
	out = append(out, panels[:start]...)
	return out
}

func (s *Server) refreshOutputLive(ctx context.Context) {
	s.OutputRefreshMu.Lock()
	defer s.OutputRefreshMu.Unlock()
	// Bound the serialized eligibility/admission sweep. Existing collectors keep
	// their own work deadline and release ownership only after lock cleanup.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return
	}
	panels, err := (readyworker.SQLSource{DB: s.DB}).EligibleReadyPanels(ctx)
	if err != nil {
		return
	}
	if len(panels) == 0 {
		return
	}
	s.OutputRefreshAfter = scheduleOutputPanels(panels, s.OutputRefreshAfter, func(p readyworker.Panel) bool {
		return s.waitPanelOutputRefresh(ctx, p)
	})
}

// waitPanelOutputRefresh fills slots freed during this sweep rather than
// leaving them idle until another ticker event. Only one dispatcher waits.
func (s *Server) waitPanelOutputRefresh(ctx context.Context, panel readyworker.Panel) bool {
	for ctx.Err() == nil {
		if s.startPanelOutputRefresh(ctx, panel) {
			return true
		}
		s.OutputPanelMu.Lock()
		running := s.OutputPanelRun[panel.ID]
		sem, done := s.OutputRefreshSem, s.OutputRefreshDone
		available := sem != nil && len(sem) < cap(sem)
		s.OutputPanelMu.Unlock()
		if running || sem == nil {
			return false
		}
		// Completion can precede this inspection. Capacity, not the coalesced
		// notification, is authoritative and prevents a lost wakeup.
		if available {
			continue
		}
		select {
		case <-ctx.Done():
			return false
		case <-done:
		}
	}
	return false
}

func (s *Server) startPanelOutputRefresh(parent context.Context, panel readyworker.Panel) bool {
	s.OutputPanelMu.Lock()
	if parent.Err() != nil {
		s.OutputPanelMu.Unlock()
		return false
	}
	if s.OutputRefreshDone == nil {
		s.OutputRefreshDone = make(chan struct{}, 1)
	}
	if s.OutputPanelRun == nil {
		s.OutputPanelRun = map[string]bool{}
	}
	if s.OutputRefreshSem == nil {
		// Each collector pins a SQL connection while holding the config lock
		// and needs another for nested reads. Reserve headroom for API work.
		limit := 8
		if s.DB != nil {
			if budget := s.DB.Stats().MaxOpenConnections; budget > 0 {
				limit = min(limit, budget/3)
			}
		}
		if limit < 1 {
			s.OutputPanelMu.Unlock()
			return false
		}
		s.OutputRefreshSem = make(chan struct{}, limit)
	}
	if s.OutputPanelRun[panel.ID] {
		s.OutputPanelMu.Unlock()
		return false
	}
	select {
	case s.OutputRefreshSem <- struct{}{}:
		s.OutputPanelRun[panel.ID] = true
		s.OutputPanelMu.Unlock()
	default:
		s.OutputPanelMu.Unlock()
		return false
	}

	go func() {
		defer func() {
			s.OutputPanelMu.Lock()
			<-s.OutputRefreshSem
			delete(s.OutputPanelRun, panel.ID)
			select {
			case s.OutputRefreshDone <- struct{}{}:
			default:
			}
			s.OutputPanelMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		defer cancel()
		_ = s.refreshPanelOutput(ctx, panel)
	}()
	return true
}

// scheduleOutputPanels uses the stable panel ID order, so deletion/insertion
// cannot reinterpret an index as a different continuation point.
func scheduleOutputPanels(panels []readyworker.Panel, after string, admit func(readyworker.Panel) bool) string {
	if len(panels) == 0 {
		return after
	}
	panels = append([]readyworker.Panel(nil), panels...)
	sort.Slice(panels, func(i, j int) bool { return panels[i].ID < panels[j].ID })
	start := sort.Search(len(panels), func(i int) bool { return panels[i].ID > after })
	if start == len(panels) {
		start = 0
	}
	for _, p := range rotateOutputPanels(panels, start) {
		if admit(p) {
			after = p.ID
		}
	}
	return after
}
