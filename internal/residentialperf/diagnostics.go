package residentialperf

// failureStage permits only operation names, never raw panel errors or URLs.
func failureStage(stage string) string {
	switch stage {
	case "client_inventory", "panel_snapshot", "settings_read", "ownership", "plan",
		"policy", "runtime_apply", "inventory_verify", "receipt_commit":
		return stage
	default:
		return "unspecified"
	}
}

// TuningProgressSQL is a read-only correlated expression for the admin status
// query's experiment e and routing control c. It uses the same freshness and
// surviving-target predicates as completion; it cannot authorize a transition.
const TuningProgressSQL = `CASE WHEN e.tuning IS NULL THEN NULL ELSE (
 SELECT json_build_object(
 'surviving',count(*),
 'verified',count(*) FILTER(WHERE COALESCE((` + tuningFresh + `
  AND t.tuning_id=(e.tuning->>'id')::uuid AND t.tuning_generation=t.generation),false)),
 'pending',count(*) FILTER(WHERE NOT COALESCE((` + tuningFresh + `
  AND t.tuning_id=(e.tuning->>'id')::uuid AND t.tuning_generation=t.generation),false)),
 'non_serving',count(*) FILTER(WHERE NOT COALESCE(pi.enabled AND a.enabled AND a.deletion_requested_at IS NULL
  AND d.state IN('READY','EXPIRING') AND (d.expires_at IS NULL OR d.expires_at>clock_timestamp()),false)),
 'credential_blocked',count(*) FILTER(WHERE a.provider_state='TOKEN_INVALID'),
 'missing_targets',(SELECT count(*) FROM residential_performance_panels owned
  JOIN panel_instances op ON op.id=owned.panel_id JOIN droplets od ON od.id=op.droplet_id
  WHERE owned.experiment_id=e.id AND od.state<>'DELETED'
  AND NOT EXISTS(SELECT 1 FROM residential_performance_targets ot WHERE ot.experiment_id=e.id AND ot.panel_id=owned.panel_id)))
 FROM residential_performance_targets t
 LEFT JOIN residential_performance_panels p ON p.panel_id=t.panel_id
 LEFT JOIN panel_routing_state r ON r.panel_id=t.panel_id
 JOIN panel_instances pi ON pi.id=t.panel_id JOIN droplets d ON d.id=pi.droplet_id
 JOIN accounts a ON a.id=pi.account_id
 WHERE t.experiment_id=e.id AND ` + tuningLive + `
 AND (NOT (e.tuning->>'phase'='TESTING' OR (e.tuning->>'phase'='RESTORING' AND e.tuning->>'restore_scope'='selected'))
  OR t.panel_id::text IN(SELECT jsonb_array_elements_text(e.tuning->'panels')))
 ) END`

// AssignmentFreshSQL is shared by display counts and the completion gate.
const AssignmentFreshSQL = tuningFresh

// AssignmentBlockerSQL reports why this exact assignment lacks native proof.
// It is diagnostic only and never advances recovery or changes policy.
const AssignmentBlockerSQL = `CASE
 WHEN NOT (` + tuningLive + `) THEN 'RETIRED'
 WHEN p.panel_id IS NULL THEN 'ASSIGNMENT_MISSING'
 WHEN p.experiment_id IS DISTINCT FROM t.experiment_id OR p.generation IS DISTINCT FROM t.generation OR p.applied_generation IS DISTINCT FROM t.generation THEN 'GENERATION_UNVERIFIED'
 WHEN t.state<>'APPLIED' THEN 'ASSIGNMENT_PENDING'
 WHEN r.state IS DISTINCT FROM 'APPLIED' THEN 'ROUTING_UNVERIFIED'
 WHEN r.revision IS DISTINCT FROM c.revision OR r.performance_generation IS DISTINCT FROM t.generation THEN 'ROUTING_GENERATION_CHANGED'
 WHEN p.verified_at IS NULL OR r.verified_at IS NULL THEN 'PROOF_MISSING'
 WHEN p.verified_at>clock_timestamp() OR r.verified_at>clock_timestamp() THEN 'PROOF_TIME_INVALID'
 WHEN p.verified_at<=clock_timestamp()-interval '60 seconds' OR r.verified_at<=clock_timestamp()-interval '60 seconds' THEN 'PROOF_STALE'
 ELSE '' END`
