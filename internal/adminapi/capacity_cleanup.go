package adminapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/http"
	"sync"
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
type cleanupJob struct {
	ID         string          `json:"id"`
	Status     string          `json:"status"`
	Total      int             `json:"total"`
	Completed  int             `json:"completed"`
	Succeeded  int             `json:"succeeded"`
	Failed     int             `json:"failed"`
	Results    []cleanupResult `json:"results"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
}

type cleanupResult struct {
	PanelID string `json:"panel_id"`
	Deleted int    `json:"deleted"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) deleteAllCapacityClients(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if _, err := s.DB.ExecContext(
		ctx,
		"UPDATE global_config_policies SET enabled=false,updated_at=now() WHERE policy_key='reality'",
	); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}

	panels, err := (readyworker.SQLSource{DB: s.DB}).EligibleReadyPanels(ctx)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}

	id := fmt.Sprintf("%d", time.Now().UnixNano())

	job := &cleanupJob{
		ID:        id,
		Status:    "queued",
		Total:     len(panels),
		Results:   make([]cleanupResult, len(panels)),
		StartedAt: time.Now(),
	}

	s.CleanupMu.Lock()
	s.CleanupJobs[id] = job
	s.CleanupMu.Unlock()

	go s.runCleanupJob(id, panels)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id": id,
		"status": "queued",
		"total":  len(panels),
	})
}

func (s *Server) cleanupJobStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	s.CleanupMu.RLock()
	job, ok := s.CleanupJobs[id]

	if !ok {
		s.CleanupMu.RUnlock()
		writeJSON(w, 404, map[string]any{"error": "not_found"})
		return
	}

	raw, _ := json.Marshal(job)
	s.CleanupMu.RUnlock()

	var result any
	_ = json.Unmarshal(raw, &result)

	writeJSON(w, 200, result)
}

func (s *Server) runCleanupJob(id string, panels []readyworker.Panel) {
	ctx := context.Background()

	s.CleanupMu.Lock()
	s.CleanupJobs[id].Status = "running"
	s.CleanupMu.Unlock()

	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup

	for i, panel := range panels {
		wg.Add(1)

		go func(i int, panel readyworker.Panel) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			result := cleanupResult{PanelID: panel.ID}

			panelCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			var err error
			result.Deleted, err = s.clearPanelClients(panelCtx, panel)
			cancel()

			if err != nil {
				result.Error = err.Error()
			}

			s.CleanupMu.Lock()

			job := s.CleanupJobs[id]
			job.Results[i] = result
			job.Completed++

			if result.Error == "" {
				job.Succeeded++
			} else {
				job.Failed++
			}
			// Cleanup job state is reported by CleanupJobs. Runtime capacity snapshots
			// remain observation-owned by usercapacity reconciliation; never synthesize
			// active counts or overwrite its health error from an admin operation.

			s.CleanupMu.Unlock()
		}(i, panel)
	}

	wg.Wait()

	now := time.Now()

	s.CleanupMu.Lock()
	job := s.CleanupJobs[id]
	job.Status = "done"
	job.FinishedAt = &now
	s.CleanupMu.Unlock()
}

func (s *Server) clearPanelClientsFast(ctx context.Context, p readyworker.Panel) (bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT remote_id,payload FROM inbound_structural_snapshots WHERE panel_id=$1 AND port = ANY($2) ORDER BY remote_id`, p.ID, pq.Array([]int{443, 7231, 1212}))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	type item struct {
		id      int64
		payload []byte
	}
	items := []item{}
	for rows.Next() {
		var x item
		if err = rows.Scan(&x.id, &x.payload); err != nil {
			return false, err
		}
		items = append(items, x)
	}
	if len(items) != 3 {
		return false, nil
	}
	rt, err := (sanaei.RuntimeFactory{DB: s.DB, Secrets: s.Container.Secrets, Timeout: 20 * time.Second}).Open(ctx, p.ID)
	if err != nil {
		return true, err
	}
	for _, x := range items {
		var payload map[string]any
		if json.Unmarshal(x.payload, &payload) != nil {
			return true, fmt.Errorf("structural payload %d", x.id)
		}
		if _, err = sanaei.UpdateInboundRaw(ctx, rt.Session.Exec, x.id, payload); err != nil {
			return true, err
		}
	}
	rt.Session.Invalidate()
	return true, nil
}

func (s *Server) clearPanelClients(ctx context.Context, p readyworker.Panel) (int, error) {
	if used, err := s.clearPanelClientsFast(ctx, p); used {
		return 0, err
	}
	return 0, fmt.Errorf("structural_snapshot_missing")
}

func (s *Server) clearPanelClientsLegacy(ctx context.Context, p readyworker.Panel) (int, error) {
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
