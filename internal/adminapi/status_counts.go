package adminapi

// These diagnostics use the provider coordinator's actual bounded read cadence.
// They never authorize a mutation; capacity admission keeps its strict TTL.
const dashboardCountsSQL = `
WITH provider_inventory AS (
 SELECT a.id account_id,a.provider_inventory_interval_seconds refresh_seconds,
 s.canonical,s.created_at,
 s.created_at<=now() AND s.created_at>now()-((a.provider_inventory_interval_seconds+30)*interval '1 second') AS current
 FROM accounts a LEFT JOIN LATERAL(
 SELECT canonical,created_at FROM provider_snapshots ps WHERE ps.account_id=a.id AND ps.canonical IS NOT NULL
 ORDER BY created_at DESC LIMIT 1)s ON true
), account_evidence AS (
 SELECT a.*,pi.created_at provider_observed_at,
 pi.canonical->'Account'->>'Status' observed_provider_status,
 ani.last_health_ok network_health_ok,ani.last_health_at network_checked_at,
 ani.rotation_started_at network_recovery_started_at,
 CASE
 WHEN a.deletion_requested_at IS NOT NULL THEN 'PENDING_DELETION'
 WHEN a.provider_state IN ('LOCKED','TOKEN_INVALID','PERMISSION_DENIED','BILLING_BLOCKED') THEN a.provider_state
 WHEN lower(pi.canonical->'Account'->>'Status')='locked'
  AND (credential.updated_at IS NULL OR credential.updated_at<=pi.created_at)
  AND (a.provider_account_observed_at IS NULL OR a.provider_account_observed_at<=pi.created_at) THEN 'LOCKED'
 WHEN COALESCE(a.provider_error_state,'')<>'' THEN a.provider_error_state
 WHEN a.provider_state='RATE_LIMITED' THEN 'RATE_LIMITED'
 WHEN a.runtime_status LIKE '%ERROR%' THEN 'RUNTIME_ERROR'
 WHEN a.runtime_status='ISOLATION_WAIT' THEN 'NETWORK_NOT_READY'
 WHEN NOT a.enabled THEN 'ACCOUNT_DISABLED'
 WHEN NOT COALESCE(pi.current,false) OR a.provider_checked_at IS NULL OR a.provider_checked_at>now()
  OR a.provider_checked_at<=now()-((a.provider_inventory_interval_seconds+30)*interval '1 second') THEN 'OBSERVATION_STALE'
 WHEN EXISTS(SELECT 1 FROM account_create_blocks b WHERE b.account_id=a.id) THEN 'CREATE_RESTRICTED'
 WHEN pi.canonical->'Account'->>'Condition'='CAPACITY_FULL'
  AND (pi.canonical->'Capacity'->>'LimitKnown')::boolean
  AND (pi.canonical->'Capacity'->>'ComputeLimit')::int>0
  AND (pi.canonical->'Capacity'->>'ComputeInUse')::int >= (pi.canonical->'Capacity'->>'ComputeLimit')::int THEN 'CAPACITY_FULL'
 WHEN COALESCE(pi.canonical->'Account'->>'Status','active') NOT IN ('active','') THEN 'PROVIDER_WARNING'
 WHEN a.provider_state='ACTIVE' THEN 'HEALTHY'
 ELSE 'UNVERIFIED' END reason
 FROM accounts a LEFT JOIN provider_inventory pi ON pi.account_id=a.id
 LEFT JOIN secrets credential ON credential.account_id=a.id AND credential.id=a.secret_ref
 LEFT JOIN account_network_identities ani ON ani.account_id=a.id
 WHERE a.deleted_at IS NULL OR a.runtime_status='DELETE_PENDING'
), live_accounts AS (
 SELECT *,CASE
 WHEN reason='PENDING_DELETION' THEN 'pending'
 WHEN reason IN ('LOCKED','TOKEN_INVALID','PERMISSION_DENIED','BILLING_BLOCKED') THEN 'broken'
 WHEN reason IN ('TRANSPORT_ERROR','PROXY_ERROR','RATE_LIMITED','NETWORK_NOT_READY','RUNTIME_ERROR') THEN 'degraded'
 WHEN reason='CAPACITY_FULL' THEN 'capacity'
 WHEN reason IN ('CREATE_RESTRICTED','PROVIDER_WARNING') THEN 'limited'
 WHEN reason='HEALTHY' THEN 'healthy'
 ELSE 'unknown' END health FROM account_evidence
), server_evidence AS (
 SELECT dr.id,dr.provider_resource_id,dr.state lifecycle_state,a.name account_name,a.provider,a.provider_state,a.provider_error_state,
 pi.created_at provider_observed_at,pi.created_at+(pi.refresh_seconds*interval '1 second') next_provider_refresh_at,
 p.last_sync_at,p.last_sync_state,p.route_state,p.route_verified_at,p.admission_blocked,p.xui_failed,p.protection_failed,
 CASE
 WHEN dr.state IN ('RETIRING','DELETING','EXPIRING') OR dr.expires_at<=now()+interval '10 seconds' OR a.deletion_requested_at IS NOT NULL THEN 'PENDING_DELETION'
 WHEN dr.state LIKE '%FAIL%' OR EXISTS(SELECT 1 FROM deployments dep WHERE dep.droplet_id=dr.id AND dep.state IN ('FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK')) THEN 'DEPLOYMENT_FAILED'
 WHEN dr.state='READY' AND p.enabled AND (p.route_state='FAILED' OR p.admission_blocked OR p.xui_failed OR p.protection_failed) THEN 'PANEL_DEGRADED'
 WHEN dr.state IN ('PROVISIONING','NEW','CREATING') THEN 'PROVISIONING'
 WHEN NOT a.enabled THEN 'ACCOUNT_DISABLED'
 WHEN a.provider_state<>'ACTIVE' OR COALESCE(a.provider_error_state,'')<>'' THEN 'PROVIDER_UNAVAILABLE'
 WHEN NOT COALESCE(pi.current,false) THEN 'PROVIDER_OBSERVATION_STALE'
 WHEN os.state IS NULL THEN 'PROVIDER_RESOURCE_UNVERIFIED'
 WHEN os.state<>'ready' THEN 'PROVIDER_NOT_READY'
 WHEN dr.state<>'READY' THEN 'LIFECYCLE_NOT_READY'
 WHEN p.id IS NULL THEN 'PANEL_MISSING'
 WHEN NOT p.enabled THEN 'PANEL_DISABLED'
 WHEN p.route_state IS DISTINCT FROM 'APPLIED' THEN 'ROUTING_UNVERIFIED'
 WHEN p.last_sync_state='FAILED' AND p.last_sync_at>now()-interval '2 minutes' THEN 'PANEL_SYNC_FAILED'
 WHEN p.last_sync_state IS DISTINCT FROM 'COMPLETED' OR p.last_sync_at IS NULL OR p.last_sync_at>now() OR p.last_sync_at<=now()-interval '2 minutes' THEN 'PANEL_OBSERVATION_STALE'
 ELSE 'HEALTHY' END reason
 FROM droplets dr JOIN accounts a ON a.id=dr.account_id
 LEFT JOIN provider_inventory pi ON pi.account_id=a.id
 LEFT JOIN LATERAL(
 SELECT x->>'State' state FROM jsonb_array_elements(CASE WHEN jsonb_typeof(pi.canonical->'Inventory'->'Servers')='array' THEN pi.canonical->'Inventory'->'Servers' ELSE '[]'::jsonb END)x
 WHERE x->>'ID'=dr.provider_resource_id LIMIT 1)os ON true
 LEFT JOIN LATERAL(
 SELECT p.id,p.enabled,rs.state route_state,rs.verified_at route_verified_at,
 pn.verified_status->>'admission_blocked'='true' admission_blocked,
 pn.verified_status->>'enabled'='true' AND pn.verified_status->>'xui_state' IN ('failed','inactive','active_xray_failed') xui_failed,
 pn.desired_enabled AND pn.state IN ('ERROR','UNSUPPORTED') protection_failed,
 latest.state last_sync_state,latest.finished_at last_sync_at
 FROM panel_instances p LEFT JOIN panel_routing_state rs ON rs.panel_id=p.id
 LEFT JOIN server_protection_nodes pn ON pn.panel_id=p.id
 LEFT JOIN LATERAL(SELECT state,finished_at FROM panel_inventory_syncs s WHERE s.panel_id=p.id AND s.finished_at IS NOT NULL ORDER BY finished_at DESC,s.id DESC LIMIT 1)latest ON true
 WHERE p.droplet_id=dr.id ORDER BY p.created_at DESC,p.id DESC LIMIT 1)p ON true
 WHERE dr.state<>'DELETED'
), live_servers AS (
 SELECT *,CASE reason WHEN 'HEALTHY' THEN 'active' WHEN 'PENDING_DELETION' THEN 'pending'
 WHEN 'DEPLOYMENT_FAILED' THEN 'broken' WHEN 'PANEL_DEGRADED' THEN 'degraded' ELSE 'inactive' END health FROM server_evidence
)
SELECT json_build_object(
 'digitalocean_accounts',(SELECT count(*) FROM live_accounts WHERE provider='digitalocean'),
 'vultr_accounts',(SELECT count(*) FROM live_accounts WHERE provider='vultr'),
 'upcloud_accounts',(SELECT count(*) FROM live_accounts WHERE provider='upcloud'),
 'aws_accounts',(SELECT count(*) FROM live_accounts WHERE provider='aws'),
 'ovh_accounts',(SELECT count(*) FROM live_accounts WHERE provider='ovh'),
 'linode_accounts',(SELECT count(*) FROM live_accounts WHERE provider='linode'),
 'enabled_accounts',(SELECT count(*) FROM live_accounts WHERE enabled),
 'healthy_accounts',(SELECT count(*) FROM live_accounts WHERE health='healthy'),
 'capacity_full_accounts',(SELECT count(*) FROM live_accounts WHERE health='capacity'),
 'broken_accounts',(SELECT count(*) FROM live_accounts WHERE health='broken'),
 'degraded_accounts',(SELECT count(*) FROM live_accounts WHERE health='degraded'),
 'account_health_total',(SELECT count(*) FROM live_accounts),
 'account_health_truncated',(SELECT count(*)>100 FROM live_accounts),
 'account_health',COALESCE((SELECT json_agg(row_to_json(detail)) FROM(
 SELECT id,name,provider,health,reason,provider_state,provider_error_state,runtime_status,provider_state_at,provider_checked_at,provider_error_at,provider_refresh_attempt_at,provider_account_status,provider_account_observed_at,provider_observed_at,observed_provider_status,network_health_ok,network_checked_at,network_recovery_started_at
 FROM live_accounts ORDER BY CASE health WHEN 'broken' THEN 0 WHEN 'degraded' THEN 1 WHEN 'limited' THEN 2 WHEN 'unknown' THEN 3 ELSE 4 END,name,id LIMIT 100)detail),'[]'::json),
 'limited_accounts',(SELECT count(*) FROM live_accounts WHERE health='limited'),
 'unknown_accounts',(SELECT count(*) FROM live_accounts WHERE health='unknown'),
 'pending_deletion_accounts',(SELECT count(*) FROM live_accounts WHERE health='pending'),
 'active_servers',(SELECT count(*) FROM live_servers WHERE health='active'),
 'inactive_servers',(SELECT count(*) FROM live_servers WHERE health='inactive'),
 'broken_servers',(SELECT count(*) FROM live_servers WHERE health='broken'),
 'degraded_servers',(SELECT count(*) FROM live_servers WHERE health='degraded'),
 'pending_deletion_servers',(SELECT count(*) FROM live_servers WHERE health='pending'),
 'total_servers',(SELECT count(*) FROM live_servers),
 'provisioning_servers',(SELECT count(*) FROM live_servers WHERE reason='PROVISIONING'),
 'provider_stale_servers',(SELECT count(*) FROM live_servers WHERE reason='PROVIDER_OBSERVATION_STALE'),
 'panel_attention_servers',(SELECT count(*) FROM live_servers WHERE health='degraded'),
 'server_health',COALESCE((SELECT json_agg(row_to_json(detail)) FROM(
 SELECT id,account_name,provider,provider_state,provider_error_state,provider_resource_id,lifecycle_state,health,reason,provider_observed_at,next_provider_refresh_at,last_sync_at,last_sync_state,route_state,route_verified_at,admission_blocked,xui_failed,protection_failed
 FROM live_servers WHERE health<>'active' ORDER BY account_name,id LIMIT 100)detail),'[]'::json),
 'active_deployments',(SELECT count(*) FROM deployments d WHERE ` + activeBuildPredicate + `),
 'live_workers',(SELECT count(*) FROM worker_heartbeats WHERE last_seen_at>now()-interval '30 seconds'),
 'observed_at',now()
)`
