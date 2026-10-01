package app

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func (c Container) recordVultrSaturation(ctx context.Context, accountID string, compute providers.ComputeDriver, createErr error) {
	if !providers.IsClass(createErr, providers.ErrorCapacity) || compute == nil {
		return
	}
	servers, err := compute.ListServers(ctx)
	if err != nil {
		log.Printf("vultr saturation inventory refresh account=%s: %v", accountID, err)
		return
	}
	limit := len(servers)
	if limit < 1 {
		log.Printf("vultr saturation ignored zero inventory account=%s", accountID)
		return
	}
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "capacity-saturation:"+accountID); err != nil {
		return
	}
	var oldLimit int
	_ = tx.QueryRowContext(ctx, `SELECT COALESCE((canonical->'Capacity'->>'ComputeLimit')::int,0) FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, accountID).Scan(&oldLimit)
	if _, err = tx.ExecContext(ctx, `INSERT INTO provider_capacity_observations(account_id,compute_limit,source,observed_at,updated_at,probe_after,probe_in_flight) VALUES($1,$2,'vultr_api_saturation',now(),now(),now()+interval '15 minutes',false) ON CONFLICT(account_id) DO UPDATE SET compute_limit=EXCLUDED.compute_limit,source=EXCLUDED.source,observed_at=now(),updated_at=now(),probe_after=now()+interval '15 minutes',probe_in_flight=false`, accountID, limit); err != nil {
		return
	}
	capacityJSON, _ := json.Marshal(providers.Capacity{ComputeLimit: limit, LimitKnown: true, ComputeInUse: limit, ObservedAt: time.Now().UTC()})
	var snapshotID string
	err = tx.QueryRowContext(ctx, `INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical,created_at) SELECT gen_random_uuid(),account_id,provider,version,data,jsonb_set(COALESCE(canonical,'{}'::jsonb),'{Capacity}',$2::jsonb,true),now() FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1 RETURNING id::text`, accountID, capacityJSON).Scan(&snapshotID)
	if err != nil {
		return
	}
	if oldLimit > 0 && oldLimit != limit {
		if _, err = tx.ExecContext(ctx, `INSERT INTO account_capacity_events(account_id,old_limit,new_limit,delta,snapshot_id) VALUES($1,$2,$3,$4,$5)`, accountID, oldLimit, limit, limit-oldLimit, snapshotID); err != nil {
			return
		}
	}
	if err = tx.Commit(); err != nil {
		return
	}
	log.Printf("vultr saturation proven account=%s limit=%d source=vultr_api_saturation", accountID, limit)
}

func (c Container) handleVultrCreateError(ctx context.Context, accountID string, compute providers.ComputeDriver, createErr error) {
	if providers.IsClass(createErr, providers.ErrorCapacity) {
		c.recordVultrSaturation(ctx, accountID, compute, createErr)
		return
	}
	_, _ = c.DB.ExecContext(ctx, "UPDATE provider_capacity_observations SET source='vultr_api_saturation',probe_in_flight=false,probe_after=now()+interval '2 minutes',updated_at=now() WHERE account_id=$1 AND source='vultr_api_probe' AND probe_in_flight=true", accountID)
}

func (c Container) recordVultrProbeSuccess(ctx context.Context, accountID string) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "capacity-probe-success:"+accountID); err != nil {
		return
	}
	var oldLimit int
	if err = tx.QueryRowContext(ctx, "SELECT compute_limit FROM provider_capacity_observations WHERE account_id=$1 AND source='vultr_api_probe' AND probe_in_flight=true FOR UPDATE", accountID).Scan(&oldLimit); err != nil {
		return
	}
	newLimit := oldLimit + 1
	if _, err = tx.ExecContext(ctx, "UPDATE provider_capacity_observations SET compute_limit=$2,source='vultr_api_probe_success',observed_at=now(),updated_at=now(),probe_after=now(),probe_in_flight=false WHERE account_id=$1", accountID, newLimit); err != nil {
		return
	}
	capacityJSON, _ := json.Marshal(providers.Capacity{ComputeLimit: newLimit, LimitKnown: true, ComputeInUse: newLimit, ObservedAt: time.Now().UTC()})
	var snapshotID string
	if err = tx.QueryRowContext(ctx, "INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical,created_at) SELECT gen_random_uuid(),account_id,provider,version,data,jsonb_set(COALESCE(canonical,'{}'::jsonb),'{Capacity}',$2::jsonb,true),now() FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1 RETURNING id::text", accountID, capacityJSON).Scan(&snapshotID); err != nil {
		return
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO account_capacity_events(account_id,old_limit,new_limit,delta,snapshot_id) VALUES($1,$2,$3,$4,$5)", accountID, oldLimit, newLimit, 1, snapshotID); err != nil {
		return
	}
	if err = tx.Commit(); err != nil {
		return
	}
	log.Printf("vultr capacity probe succeeded account=%s admitted_limit=%d", accountID, newLimit)
}
