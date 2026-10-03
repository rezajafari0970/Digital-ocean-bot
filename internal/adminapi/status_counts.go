package adminapi

// Counts are mutually exclusive within each health breakdown. No stale snapshot
// is promoted to "healthy"; unknown observations remain visible as unverified.
const dashboardCountsSQL = `
WITH live_accounts AS (
 SELECT *, CASE
 WHEN deletion_requested_at IS NOT NULL THEN 'pending'
 WHEN provider_state IN ('LOCKED','TOKEN_INVALID','PERMISSION_DENIED','BILLING_BLOCKED')
   OR COALESCE(provider_error_state,'')<>'' OR runtime_status LIKE '%ERROR%' THEN 'broken'
 WHEN provider_state='ACTIVE' AND provider_checked_at>now()-interval '2 minutes' THEN 'healthy'
 ELSE 'unknown' END health
 FROM accounts WHERE deleted_at IS NULL OR runtime_status='DELETE_PENDING'
), live_servers AS (
 SELECT dr.id, CASE
 WHEN dr.state IN ('RETIRING','DELETING','EXPIRING') OR dr.expires_at<=now()+interval '10 seconds'
   OR a.deletion_requested_at IS NOT NULL THEN 'pending'
 WHEN dr.state LIKE '%FAIL%' OR EXISTS(SELECT 1 FROM user_capacity_snapshots u JOIN panel_instances pi ON pi.id=u.panel_id WHERE pi.droplet_id=dr.id AND u.observed_at>now()-interval '30 seconds' AND COALESCE(u.last_error,'')<>'') OR EXISTS(
   SELECT 1 FROM deployments dep WHERE dep.droplet_id=dr.id AND dep.state IN ('FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK')
 ) THEN 'broken'
 WHEN dr.state='READY' AND a.provider_state='ACTIVE' AND EXISTS(
   SELECT 1 FROM panel_instances pi JOIN deployments dep ON dep.droplet_id=pi.droplet_id
   WHERE pi.droplet_id=dr.id AND pi.enabled AND dep.state='PANEL_COMPLETE'
   AND (EXISTS(SELECT 1 FROM user_capacity_snapshots u WHERE u.panel_id=pi.id AND u.observed_at>now()-interval '30 seconds' AND COALESCE(u.last_error,'')='')
     OR EXISTS(SELECT 1 FROM panel_inventory_syncs i WHERE i.panel_id=pi.id AND i.state='COMPLETED' AND i.finished_at>now()-interval '30 seconds'))
 ) THEN 'active'
 ELSE 'inactive' END health
 FROM droplets dr JOIN accounts a ON a.id=dr.account_id WHERE dr.state<>'DELETED'
)
SELECT json_build_object(
 'digitalocean_accounts',(SELECT count(*) FROM live_accounts WHERE provider='digitalocean'),
 'vultr_accounts',(SELECT count(*) FROM live_accounts WHERE provider='vultr'),
 'enabled_accounts',(SELECT count(*) FROM live_accounts WHERE enabled),
 'healthy_accounts',(SELECT count(*) FROM live_accounts WHERE health='healthy'),
 'broken_accounts',(SELECT count(*) FROM live_accounts WHERE health='broken'),
 'unknown_accounts',(SELECT count(*) FROM live_accounts WHERE health='unknown'),
 'pending_deletion_accounts',(SELECT count(*) FROM live_accounts WHERE health='pending'),
 'active_servers',(SELECT count(*) FROM live_servers WHERE health='active'),
 'inactive_servers',(SELECT count(*) FROM live_servers WHERE health='inactive'),
 'broken_servers',(SELECT count(*) FROM live_servers WHERE health='broken'),
 'pending_deletion_servers',(SELECT count(*) FROM live_servers WHERE health='pending'),
 'total_servers',(SELECT count(*) FROM live_servers),
 'active_deployments',(SELECT count(*) FROM deployments WHERE state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE')),
 'live_workers',(SELECT count(*) FROM worker_heartbeats WHERE last_seen_at>now()-interval '30 seconds'),
 'observed_at',now()
)`
