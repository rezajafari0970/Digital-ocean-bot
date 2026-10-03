package usercapacity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

type Canary struct {
	ID             string
	GenerationID   string
	PanelID        string
	InboundID      int64
	TargetUsers    int
	UsersPerSecond int
	State          string
	ExpiresAt      time.Time
}

func (s Service) effectiveTargetRate(ctx context.Context, panelID string, inboundID int64, target, rate int) (int, int, error) {
	var t, r int
	err := s.DB.QueryRowContext(ctx,
		"SELECT target_users,users_per_second FROM user_capacity_canaries WHERE panel_id=$1 AND inbound_id=$2 AND state='ACTIVE' AND expires_at>now()",
		panelID, inboundID,
	).Scan(&t, &r)
	if errors.Is(err, sql.ErrNoRows) {
		return target, rate, nil
	}
	if err != nil {
		return 0, 0, err
	}
	return t, r, nil
}

func (s Service) CreateCanary(ctx context.Context, panelID string, inboundID int64, target, rate int, ttl time.Duration) (Canary, error) {
	var out Canary
	if s.DB == nil || panelID == "" || inboundID <= 0 || target < 2 || target > 10000 ||
		rate < 1 || rate > 100 || ttl < time.Minute || ttl > 15*time.Minute {
		return out, errors.New("invalid capacity canary")
	}
	var eligible bool
	err := s.DB.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id JOIN deployments dep ON dep.droplet_id=d.id JOIN panel_inbound_inventory i ON i.panel_id=p.id AND i.remote_id=$2 WHERE p.id=$1 AND p.enabled AND a.enabled AND a.provider_state='ACTIVE' AND d.state='READY' AND dep.state='PANEL_COMPLETE' AND i.present AND i.enabled)",
		panelID, inboundID,
	).Scan(&eligible)
	if err != nil || !eligible {
		return out, errors.New("capacity canary target not eligible")
	}

	marker := fmt.Sprintf("c%s-i%d-%d", compactPanelID(panelID), inboundID, time.Now().Unix())
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()

	var generationID string
	if err = tx.QueryRowContext(ctx,
		"INSERT INTO bulk_user_generations(panel_id,inbound_id,purpose,marker) VALUES($1,$2,'CANARY',$3) RETURNING id::text",
		panelID, inboundID, marker,
	).Scan(&generationID); err != nil {
		return out, err
	}
	expires := time.Now().Add(ttl)
	if err = tx.QueryRowContext(ctx,
		"INSERT INTO user_capacity_canaries(generation_id,panel_id,inbound_id,target_users,users_per_second,expires_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text,state,created_at",
		generationID, panelID, inboundID, target, rate, expires,
	).Scan(&out.ID, &out.State, new(time.Time)); err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	out.GenerationID, out.PanelID, out.InboundID = generationID, panelID, inboundID
	out.TargetUsers, out.UsersPerSecond, out.ExpiresAt = target, rate, expires
	return out, nil
}

func compactPanelID(id string) string {
	out := ""
	for _, r := range id {
		if r != '-' {
			out += string(r)
		}
	}
	return out
}

func (s Service) effectiveGeneration(ctx context.Context, panelID string, inboundID int64) (bulkGeneration, error) {
	var g bulkGeneration
	err := s.DB.QueryRowContext(ctx,
		"SELECT g.id::text,g.marker FROM user_capacity_canaries c JOIN bulk_user_generations g ON g.id=c.generation_id WHERE c.panel_id=$1 AND c.inbound_id=$2 AND c.state='ACTIVE' AND c.expires_at>now()",
		panelID, inboundID,
	).Scan(&g.ID, &g.Marker)
	if err == nil {
		return g, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return g, err
	}
	return s.activePolicyGeneration(ctx, panelID, inboundID)
}

func (s Service) RollbackExpiredCanary(ctx context.Context, p readyworker.Panel, runtime *sanaei.PanelRuntime) (bool, error) {
	var id, generationID string
	err := s.DB.QueryRowContext(ctx,
		"SELECT id::text,generation_id::text FROM user_capacity_canaries WHERE panel_id=$1 AND state IN ('ACTIVE','ROLLBACK_PENDING','ROLLING_BACK') AND expires_at<=now() ORDER BY expires_at LIMIT 1",
		p.ID,
	).Scan(&id, &generationID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err = s.DB.ExecContext(ctx,
		"UPDATE user_capacity_canaries SET state='ROLLING_BACK',updated_at=now() WHERE id=$1 AND state IN ('ACTIVE','ROLLBACK_PENDING')",
		id,
	); err != nil {
		return false, err
	}
	mutated, err := s.RollbackOwnedGeneration(ctx, runtime, generationID)
	if err != nil {
		_, _ = s.DB.ExecContext(context.WithoutCancel(ctx),
			"UPDATE user_capacity_canaries SET state='ROLLBACK_PENDING',last_error=$2,updated_at=now() WHERE id=$1",
			id, err.Error())
		return mutated, err
	}
	var remaining int
	if err = s.DB.QueryRowContext(ctx,
		"SELECT count(*) FROM bulk_user_ownership WHERE generation_id=$1 AND state IN ('PLANNED','ACTIVE','DELETE_PENDING')", generationID,
	).Scan(&remaining); err != nil {
		return mutated, err
	}
	if remaining == 0 {
		_, err = s.DB.ExecContext(ctx,
			"UPDATE user_capacity_canaries SET state='COMPLETED',last_error='',completed_at=now(),updated_at=now() WHERE id=$1",
			id)
	}
	return mutated, err
}
