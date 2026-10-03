package readyworker

import (
	"context"
	"database/sql"
)

type SQLSource struct{ DB *sql.DB }

func (s SQLSource) EligibleReadyPanels(ctx context.Context) ([]Panel, error) {
	if s.DB == nil {
		return nil, ErrAdapterConfig
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT pi.id::text
FROM panel_instances pi
JOIN droplets r ON r.id=pi.droplet_id
JOIN accounts a ON a.id=r.account_id
JOIN deployments d ON d.droplet_id=pi.droplet_id
JOIN xui_panel_deployments x
  ON x.droplet_id=pi.droplet_id
 AND x.generation=d.postinstall_generation
WHERE pi.enabled=true
  AND COALESCE(pi.health_state,'UNKNOWN')<>'UNHEALTHY'
  AND r.state IN ('READY','EXPIRING','RETIRING')
  AND (r.expires_at IS NULL OR r.expires_at > now()+interval '10 seconds')
  AND a.provider_state <> 'LOCKED'
  AND d.state='PANEL_COMPLETE'
  AND x.state='COMPLETED'
ORDER BY pi.id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Panel
	for rows.Next() {
		var p Panel
		if err = rows.Scan(&p.ID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s SQLSource) EligibleExpiringPanels(ctx context.Context, minutes int) ([]Panel, error) {
	if s.DB == nil {
		return nil, ErrAdapterConfig
	}
	if minutes < 1 {
		minutes = 1
	}
	if minutes > 1440 {
		minutes = 1440
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT pi.id::text FROM panel_instances pi JOIN droplets r ON r.id=pi.droplet_id JOIN accounts a ON a.id=r.account_id JOIN deployments d ON d.droplet_id=pi.droplet_id JOIN xui_panel_deployments x ON x.droplet_id=pi.droplet_id AND x.generation=d.postinstall_generation WHERE pi.enabled=true AND COALESCE(pi.health_state,'UNKNOWN')<>'UNHEALTHY' AND r.state IN ('READY','EXPIRING','RETIRING') AND r.expires_at IS NOT NULL AND r.expires_at > now()+interval '10 seconds' AND r.expires_at <= now()+($1*interval '1 minute') AND a.provider_state <> 'LOCKED' AND d.state='PANEL_COMPLETE' AND x.state='COMPLETED' ORDER BY pi.id`, minutes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Panel
	for rows.Next() {
		var p Panel
		if err = rows.Scan(&p.ID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
