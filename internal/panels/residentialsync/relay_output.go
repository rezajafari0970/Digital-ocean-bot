package residentialsync

// SQL uses the existing Output aliases p and rs. Direct proxy health must never
// certify a relay: bind its current plan, fresh native observation, exact
// managed SOCKS endpoint and donor lifecycle. One surviving route may be published
// while a lost third slot is automatically replenished; three are required by
// the separate full-production acceptance gate.
const RelayOutputHealthySQL = `EXISTS(
 SELECT 1 FROM panel_relay_assignments ra
 JOIN panel_instances dp ON dp.id=ra.donor_panel_id
 JOIN droplets dd ON dd.id=ra.donor_droplet_id AND dd.id=dp.droplet_id
 JOIN accounts da ON da.id=ra.donor_account_id AND da.id=dp.account_id
 JOIN panel_routing_state ds ON ds.panel_id=dp.id
 WHERE ra.receiver_panel_id=p.id AND ra.selected AND ra.applied AND ra.alive
 AND ra.receiver_plan_hash=rs.plan_hash AND ra.observed_at>now()-interval '20 seconds'
 AND ra.last_success_at>now()-interval '35 seconds'
 AND dp.enabled AND da.enabled AND da.deletion_requested_at IS NULL AND da.deleted_at IS NULL
 AND da.provider_state='ACTIVE' AND dd.state='READY' AND(dd.expires_at IS NULL OR dd.expires_at>now()+interval '60 seconds')
 AND ds.state='APPLIED' AND NOT ds.relay_mode AND ds.plan_hash=ra.donor_plan_hash
 AND ds.category_digest=rs.category_digest AND ds.verified_at>now()-interval '60 seconds'
 AND EXISTS(SELECT 1 FROM panel_relay_endpoints ep
  WHERE ep.panel_id=dp.id AND ep.enabled AND ep.state='APPLIED' AND ep.plan_hash=ds.plan_hash
  AND ep.transport_hash=ra.transport_hash AND ep.secret_ref=ra.secret_ref AND ep.verified_at>now()-interval '60 seconds')
 AND NOT EXISTS(SELECT 1 FROM panel_cleanup_targets ct JOIN panel_cleanup_jobs cj ON cj.id=ct.job_id WHERE ct.panel_id=dp.id AND cj.state NOT IN('SUCCEEDED','CANCELLED'))
 AND NOT EXISTS(SELECT 1 FROM server_protection_nodes pn WHERE pn.panel_id=dp.id AND
  (pn.verified_status->>'admission_blocked'='true' OR(pn.verified_status->>'enabled'='true' AND pn.verified_status->>'xui_state' IN('failed','inactive','active_xray_failed'))))
)`
