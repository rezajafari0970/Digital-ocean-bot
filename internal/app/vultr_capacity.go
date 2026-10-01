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
	if _, err = tx.ExecContext(ctx, `INSERT INTO provider_capacity_observations(account_id,compute_limit,source,observed_at,updated_at) VALUES($1,$2,'vultr_api_saturation',now(),now()) ON CONFLICT(account_id) DO UPDATE SET compute_limit=EXCLUDED.compute_limit,source=EXCLUDED.source,observed_at=now(),updated_at=now()`, accountID, limit); err != nil {
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
