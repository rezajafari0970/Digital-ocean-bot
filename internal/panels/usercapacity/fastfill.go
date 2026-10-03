package usercapacity

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

// FastFill plans durable work only. The bool describes a committed plan, never
// a successful remote mutation. The worker owns execution and fresh verification.
func (s Service) FastFill(ctx context.Context, p readyworker.Panel, runtime *sanaei.PanelRuntime, wanted map[int]bool, target int, quota int64, life, limit, rate int) (bool, error) {
	if runtime == nil || runtime.Session == nil || s.DB == nil || target <= 0 || rate <= 0 {
		return false, nil
	}
	var inboundID int64
	var chunk int
	err := s.DB.QueryRowContext(ctx, `SELECT inbound_id,max_batch_size FROM bulk_client_execution_gate WHERE singleton AND enabled AND NOT kill_switch AND panel_id=$1 AND remaining_batches>0 AND expires_at>now()`, p.ID).Scan(&inboundID, &chunk)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	runtime.Session.Invalidate()
	raws, err := runtime.Session.Snapshot(ctx)
	if err != nil {
		return false, err
	}
	snap, err := sanaei.InventoryFromRaw(p.ID, raws)
	if err != nil {
		return false, err
	}
	for _, in := range snap.Records {
		if in.RemoteID != inboundID || !wanted[in.Port] || !in.Enabled || in.Protocol != "vless" || in.Transport != "tcp" || in.Security != "reality" {
			continue
		}
		effectiveTarget, effectiveRate, err := s.effectiveTargetRate(ctx, p.ID, inboundID, target, rate)
		if err != nil {
			return false, err
		}
		var blocked bool
		err = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM client_mutation_jobs WHERE panel_id=$1 AND inbound_id=$2 AND kind='BULK_CREATE' AND state IN ('PENDING','RUNNING','FAILED')) OR EXISTS(SELECT 1 FROM bulk_user_ownership o JOIN bulk_user_generations g ON g.id=o.generation_id WHERE g.panel_id=$1 AND g.inbound_id=$2 AND o.state IN ('PLANNED','DELETE_PENDING'))`, p.ID, inboundID).Scan(&blocked)
		if err != nil || blocked {
			return false, err
		}
		deficit := effectiveTarget - in.ClientCount
		if deficit <= 0 {
			return false, nil
		}
		if deficit > chunk {
			deficit = chunk
		}
		n, err := s.durableAllowance(ctx, p.ID, inboundID, effectiveRate, deficit, time.Now())
		if err != nil || n == 0 {
			return false, err
		}
		generation, err := s.effectiveGeneration(ctx, p.ID, inboundID)
		if err != nil {
			return false, err
		}
		expiry := int64(0)
		if life > 0 {
			expiry = time.Now().Add(time.Duration(life) * time.Second).UnixMilli()
		}
		payload := clientops.BulkPayload{GenerationID: generation.ID, TargetUsers: effectiveTarget}
		for i := 0; i < n; i++ {
			id, err := sanaei.UUIDv4()
			if err != nil {
				return false, err
			}
			payload.Clients = append(payload.Clients, sanaei.Client{ID: id, Email: ownershipEmail(generation.Marker, id), Enable: true, TotalGB: quota, ExpiryTime: expiry, LimitHWID: limit, Flow: "xtls-rprx-vision"})
		}
		_, planned, err := (clientops.Journal{DB: s.DB}).ReserveBulk(ctx, runtime.AccountID, p.ID, inboundID, payload)
		return planned, err
	}
	return false, nil
}
