package adminapi

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/residentialsync"
	"time"
)

type trialRelayPanel struct {
	PanelID  string `json:"panel_id"`
	State    string `json:"state"`
	Selected int    `json:"selected"`
	Healthy  int    `json:"healthy"`
}
type trialRelayStatus struct {
	Status                string            `json:"status"`
	Panels                []trialRelayPanel `json:"panels"`
	Selected              int               `json:"selected"`
	Healthy               int               `json:"healthy"`
	BelowMinimum          int               `json:"below_minimum"`
	CheckedAt             time.Time         `json:"checked_at"`
	AdmitLifetimeSeconds  int               `json:"admit_lifetime_seconds"`
	RetireLifetimeSeconds int               `json:"retire_lifetime_seconds"`
}

func (s *Server) trialRelayStatus(ctx context.Context, account string) (*trialRelayStatus, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT p.id::text,rs.state,
 (SELECT count(*) FROM panel_relay_assignments ra WHERE ra.receiver_panel_id=p.id AND ra.selected AND ra.applied AND ra.receiver_plan_hash=rs.plan_hash),
 CASE WHEN rs.state='APPLIED' THEN `+residentialsync.RelayHealthyCountSQL+` ELSE 0 END
 FROM panel_instances p JOIN droplets dr ON dr.id=p.droplet_id JOIN panel_routing_state rs ON rs.panel_id=p.id
 WHERE p.account_id=$1 AND p.enabled AND dr.state='READY' AND (dr.expires_at IS NULL OR dr.expires_at>now()) AND rs.relay_mode
 ORDER BY p.id`, account)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := &trialRelayStatus{Status: "healthy", Panels: []trialRelayPanel{}, CheckedAt: time.Now().UTC(), AdmitLifetimeSeconds: 600, RetireLifetimeSeconds: 300}
	zero := false
	for rows.Next() {
		var p trialRelayPanel
		if e = rows.Scan(&p.PanelID, &p.State, &p.Selected, &p.Healthy); e != nil {
			return nil, e
		}
		out.Panels = append(out.Panels, p)
		out.Selected += p.Selected
		out.Healthy += p.Healthy
		if p.Healthy < residentialsync.RelayMinimumHealthy {
			out.BelowMinimum++
		}
		if p.Healthy == 0 {
			zero = true
		}
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out.Panels) == 0 {
		return nil, nil
	}
	if out.BelowMinimum > 0 {
		out.Status = "degraded"
	}
	if zero {
		out.Status = "unavailable"
	}
	return out, nil
}
