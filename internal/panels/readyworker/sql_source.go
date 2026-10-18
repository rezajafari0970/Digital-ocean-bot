package readyworker

import (
	"context"
	"database/sql"
)

type SQLSource struct{ DB *sql.DB }

func (s SQLSource) EligibleReadyPanels(ctx context.Context) ([]Panel, error) {
	return s.eligibleReadyPanels(ctx, nil)
}

// EligibleRealityPanels partitions work using observed public VLESS listeners.
// Both lanes still reconcile authoritative state under the same config lock.
func (s SQLSource) EligibleRealityPanels(ctx context.Context, initial bool) ([]Panel, error) {
	var unified bool
	if err := s.DB.QueryRowContext(ctx, "SELECT unified FROM config_control_authority WHERE singleton").Scan(&unified); err != nil {
		return nil, err
	}
	if unified {
		return nil, nil
	}
	return s.eligibleReadyPanels(ctx, &initial)
}
func (s SQLSource) eligibleReadyPanels(ctx context.Context, initial *bool) ([]Panel, error) {
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
  AND r.state IN ('READY','EXPIRING')
  AND (r.expires_at IS NULL OR r.expires_at > now()+interval '10 seconds')
  AND a.provider_state = 'ACTIVE' AND a.enabled AND a.deletion_requested_at IS NULL
  AND d.state='PANEL_COMPLETE'
  AND x.state='COMPLETED'
  AND ($1::boolean IS NULL OR $1 = NOT EXISTS(
    SELECT 1 FROM panel_inbound_inventory i WHERE i.panel_id=pi.id
    AND i.present AND i.enabled AND i.protocol='vless' AND i.security='reality'
    AND i.transport IN ('tcp','raw','xhttp')))
ORDER BY pi.id
`, initial)
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
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT pi.id::text FROM panel_instances pi JOIN droplets r ON r.id=pi.droplet_id JOIN accounts a ON a.id=r.account_id JOIN deployments d ON d.droplet_id=pi.droplet_id JOIN xui_panel_deployments x ON x.droplet_id=pi.droplet_id AND x.generation=d.postinstall_generation WHERE pi.enabled=true AND r.state IN ('READY','EXPIRING') AND r.expires_at IS NOT NULL AND r.expires_at > now()+interval '10 seconds' AND r.expires_at <= now()+($1*interval '1 minute') AND a.provider_state = 'ACTIVE' AND d.state='PANEL_COMPLETE' AND x.state='COMPLETED' ORDER BY pi.id`, minutes)
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
