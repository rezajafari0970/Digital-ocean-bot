package app

import (
	"context"
	"encoding/json"
	"log"
)

// ReconcileLocalState repairs only relationships that are provable from local
// persisted state. It never creates or deletes provider resources.
func (c Container) ReconcileLocalState(ctx context.Context) {
	// Disabled/deleting accounts must never keep scheduler leases alive.
	// This also repairs rows created before delete disabled schedules atomically.
	_, _ = c.DB.ExecContext(ctx, `UPDATE schedules s SET enabled=false,lease_until=NULL,updated_at=now() FROM accounts a WHERE a.id=s.account_id AND a.enabled=false AND (s.enabled=true OR s.lease_until IS NOT NULL)`)
	_, _ = c.DB.ExecContext(ctx, `UPDATE droplets r SET backfill_required=false,updated_at=now() FROM accounts a WHERE a.id=r.account_id AND a.enabled=false AND r.backfill_required=true`)

	// Terminal deployments own the terminal state of their provisioning ledger.
	// Repair historical rows left behind by crashes or older workflow versions.
	_, _ = c.DB.ExecContext(ctx, `WITH done AS (
		UPDATE provision_runs pr SET state='COMPLETED',current_step='done',last_error=NULL,next_retry_at=NULL,updated_at=now()
		FROM deployments d WHERE d.account_id=pr.account_id AND d.droplet_id=pr.droplet_id
		AND d.state IN ('PANEL_COMPLETE','READY') AND pr.state<>'COMPLETED' RETURNING pr.id
	) UPDATE provision_step_attempts SET next_retry_at=NULL FROM done WHERE run_id=done.id`)
	_, _ = c.DB.ExecContext(ctx, `WITH failed AS (
		UPDATE provision_runs pr SET state='FAILED',next_retry_at=NULL,updated_at=now()
		FROM deployments d WHERE d.account_id=pr.account_id AND d.droplet_id=pr.droplet_id
		AND d.state IN ('FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK') AND pr.state NOT IN ('FAILED','COMPLETED') RETURNING pr.id
	) UPDATE provision_step_attempts SET next_retry_at=NULL FROM failed WHERE run_id=failed.id`)
	_, _ = c.DB.ExecContext(ctx, `UPDATE provision_step_attempts psa SET next_retry_at=NULL FROM provision_runs pr WHERE pr.id=psa.run_id AND pr.state IN ('COMPLETED','FAILED') AND psa.next_retry_at IS NOT NULL`)
	_, _ = c.DB.ExecContext(ctx, `
UPDATE droplets r
SET replacement_deployment_id=NULL,backfill_required=true,updated_at=now()
FROM deployments d,accounts a
WHERE r.replacement_deployment_id=d.id
  AND r.account_id=a.id
  AND r.state='DELETED'
  AND a.enabled=true
  AND d.state IN ('FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK')
  AND NOT EXISTS (
    SELECT 1 FROM droplets live
    WHERE live.id=d.droplet_id AND live.state<>'DELETED'
  )`)

	// A crashed/ambiguous Vultr capacity probe must not hold the account
	// forever. Active create operations retain the claim; otherwise an old
	// claim is safely returned to the last proven saturation/lower-bound state.
	_, _ = c.DB.ExecContext(ctx, `
UPDATE provider_capacity_observations p
SET source=CASE WHEN p.compute_limit>0 THEN 'vultr_api_saturation' ELSE 'vultr_api_lower_bound' END,
    probe_in_flight=false,
    probe_after=CASE WHEN p.compute_limit>0 THEN now() ELSE NULL END,
    updated_at=now()
FROM accounts a
WHERE a.id=p.account_id
  AND p.source='vultr_api_probe'
  AND p.probe_in_flight=true
  AND (a.enabled=false OR p.updated_at<now()-interval '30 minutes')
  AND NOT EXISTS (
    SELECT 1 FROM operations o
    WHERE o.account_id=p.account_id
      AND o.kind='CREATE_DROPLET'
      AND o.state IN ('planned','running','unknown','verifying')
  )`)

	// Link deployments to an already-known lifecycle droplet when provider IDs agree.
	_, _ = c.DB.ExecContext(ctx, `UPDATE deployments d SET droplet_id=dr.id,updated_at=now()
        FROM droplets dr WHERE d.droplet_id IS NULL AND d.provider_id IS NOT NULL
        AND dr.account_id=d.account_id AND dr.provider_resource_id=d.provider_id AND dr.state<>'DELETED'`)

	// Mirror lifecycle droplets into inventory. This is an upsert and therefore
	// safe to run repeatedly.
	rows, err := c.DB.QueryContext(ctx, `SELECT r.id::text,r.account_id::text,r.provider_resource_id,r.state,r.profile,a.provider_state,a.provider FROM droplets r JOIN accounts a ON a.id=r.account_id WHERE r.provider_resource_id IS NOT NULL AND r.state<>'DELETED'`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var did, aid, pid, state, providerState, providerName string
		var profile []byte
		if rows.Scan(&did, &aid, &pid, &state, &profile, &providerState, &providerName) != nil {
			continue
		}
		meta, _ := json.Marshal(map[string]any{"droplet_id": did, "profile": json.RawMessage(profile)})
		resourceState := "provisioning"
		if providerState != ProviderStateLocked && (state == "READY" || state == "EXPIRING" || state == "RETIRING") {
			resourceState = "active"
		}
		if providerState == ProviderStateLocked {
			resourceState = "retiring"
		}
		_, err = c.DB.ExecContext(ctx, `INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed,metadata)
            VALUES(gen_random_uuid(),$1,$5,$2,'droplet',$3,true,$4)
            ON CONFLICT(account_id,provider,type,provider_resource_id)
            DO UPDATE SET state=EXCLUDED.state,managed=true,metadata=EXCLUDED.metadata,updated_at=now()`, aid, pid, resourceState, meta, providerName)
		if err != nil {
			log.Printf("consistency inventory %s: %v", pid, err)
		}
	}
}
