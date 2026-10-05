package adminapi

// Counts are mutually exclusive within each health breakdown. No stale snapshot
// is promoted to "healthy"; unknown observations remain visible as unverified.
const dashboardCountsSQL = `
WITH provider_inventory AS (
 SELECT a.id account_id, s.canonical,s.created_at FROM accounts a
 LEFT JOIN LATERAL(SELECT canonical,created_at FROM provider_snapshots ps
 WHERE ps.account_id=a.id AND ps.canonical IS NOT NULL ORDER BY created_at DESC LIMIT 1)s ON true
), observed_servers AS (
 SELECT p.account_id,x->>'ID' provider_id,x->>'State' state,p.created_at
 FROM provider_inventory p CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(p.canonical->'Inventory'->'Servers')='array' THEN p.canonical->'Inventory'->'Servers' ELSE '[]'::jsonb END)x
 WHERE p.created_at>now()-interval '2 minutes'
), live_accounts AS (
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
 WHEN dr.state LIKE '%FAIL%' OR EXISTS(
 SELECT 1 FROM deployments dep WHERE dep.droplet_id=dr.id AND dep.state IN ('FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK')
 ) THEN 'broken'
 WHEN dr.state='READY' AND EXISTS(SELECT 1 FROM observed_servers os WHERE os.account_id=dr.account_id AND os.provider_id=dr.provider_resource_id AND os.state='ready') THEN 'active'
 ELSE 'inactive' END health
 FROM droplets dr JOIN accounts a ON a.id=dr.account_id WHERE dr.state<>'DELETED'
)
SELECT json_build_object(
 'digitalocean_accounts',(SELECT count(*) FROM live_accounts WHERE provider='digitalocean'),
 'vultr_accounts',(SELECT count(*) FROM live_accounts WHERE provider='vultr'),
 'upcloud_accounts',(SELECT count(*) FROM live_accounts WHERE provider='upcloud'),
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
 'panel_attention_servers',(SELECT count(DISTINCT pi.droplet_id) FROM panel_instances pi JOIN droplets dr ON dr.id=pi.droplet_id JOIN panel_routing_state rs ON rs.panel_id=pi.id WHERE dr.state='READY' AND pi.enabled AND rs.state='FAILED'),
 'active_deployments',(SELECT count(*) FROM deployments WHERE state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE')),
 'live_workers',(SELECT count(*) FROM worker_heartbeats WHERE last_seen_at>now()-interval '30 seconds'),
 'observed_at',now()
)`
