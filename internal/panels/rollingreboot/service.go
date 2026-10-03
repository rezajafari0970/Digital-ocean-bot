package rollingreboot

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Service struct {
	DB                 *sql.DB
	Enabled            bool
	MinReadyPerAccount int
}

func (s Service) Plan(ctx context.Context) (int, error) {
	if s.DB == nil {
		return 0, fmt.Errorf("rolling reboot db")
	}
	res, err := s.DB.ExecContext(ctx, `
INSERT INTO rolling_reboot_jobs(panel_id,account_id,droplet_id,state,updated_at)
SELECT pi.id,pi.account_id,pi.droplet_id,'PENDING',now()
FROM panel_instances pi
JOIN droplets d ON d.id=pi.droplet_id
JOIN accounts a ON a.id=d.account_id
JOIN deployments dep ON dep.droplet_id=d.id
JOIN server_runtime_snapshots rs ON rs.panel_id=pi.id
WHERE pi.enabled=true AND a.enabled=true AND a.provider_state='ACTIVE'
  AND d.state='READY' AND dep.state='PANEL_COMPLETE'
  AND rs.reboot_required=true AND rs.xui_active=true AND rs.last_error=''
  AND (d.expires_at IS NULL OR d.expires_at>now()+interval '30 minutes')
  AND (SELECT count(*) FROM droplets x WHERE x.account_id=a.id AND x.state='READY') >= $1
ON CONFLICT(panel_id) DO NOTHING
`, max(2, s.MinReadyPerAccount))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s Service) DryRun(ctx context.Context) ([]Candidate, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT j.id::text,a.name,j.panel_id::text,j.droplet_id::text,dep.host,
 (SELECT count(*) FROM droplets x WHERE x.account_id=j.account_id AND x.state='READY') ready,
 d.expires_at
FROM rolling_reboot_jobs j
JOIN accounts a ON a.id=j.account_id
JOIN droplets d ON d.id=j.droplet_id
JOIN deployments dep ON dep.droplet_id=d.id
WHERE j.state='PENDING'
ORDER BY ready DESC,d.expires_at DESC NULLS LAST,j.id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Candidate
	for rows.Next() {
		var c Candidate
		if err = rows.Scan(&c.JobID, &c.Account, &c.PanelID, &c.DropletID, &c.Host, &c.Ready, &c.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s Service) ReconcileDeferred(ctx context.Context) (int, error) {
	res, err := s.DB.ExecContext(ctx, `
UPDATE rolling_reboot_jobs j
SET state='OBSOLETE',completed_at=now(),next_retry_at=NULL,
    last_error='reboot maintenance no longer applicable',updated_at=now()
WHERE j.state='DEFERRED'
  AND (
    NOT EXISTS (SELECT 1 FROM droplets d WHERE d.id=j.droplet_id)
    OR EXISTS (SELECT 1 FROM droplets d WHERE d.id=j.droplet_id AND d.state IN ('RETIRING','DELETING','DELETED'))
    OR EXISTS (SELECT 1 FROM server_runtime_snapshots rs WHERE rs.panel_id=j.panel_id AND rs.reboot_required=false)
  )
`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s Service) Reactivate(ctx context.Context, panelID string) error {
	res, err := s.DB.ExecContext(ctx, `
UPDATE rolling_reboot_jobs j
SET state='PENDING',completed_at=NULL,next_retry_at=NULL,last_error='',boot_id_before='',updated_at=now()
FROM panel_instances pi
JOIN droplets d ON d.id=pi.droplet_id
JOIN accounts a ON a.id=d.account_id
JOIN server_runtime_snapshots rs ON rs.panel_id=pi.id
WHERE j.panel_id=pi.id AND pi.id=$1
  AND j.state IN ('DEFERRED','OBSOLETE')
  AND a.enabled=true AND a.provider_state='ACTIVE'
  AND d.state='READY' AND rs.reboot_required=true AND rs.xui_active=true AND rs.last_error=''
  AND (d.expires_at IS NULL OR d.expires_at>now()+interval '30 minutes')
`, panelID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("reboot job not eligible for reactivation")
	}
	return nil
}

type Candidate struct {
	JobID, Account, PanelID, DropletID, Host string
	Ready                                    int
	ExpiresAt                                *time.Time
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
