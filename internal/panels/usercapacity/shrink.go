package usercapacity

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func (s Service) shrinkGate(ctx context.Context, panelID string, inboundID int64) (bool, int, error) {
	var enabled bool
	var limit int
	var scopePanel sql.NullString
	var scopeInbound sql.NullInt64
	err := s.DB.QueryRowContext(ctx, "SELECT enabled,max_delete_per_inbound,panel_id::text,inbound_id FROM bulk_user_shrink_gate WHERE singleton=true").Scan(&enabled, &limit, &scopePanel, &scopeInbound)
	if err != nil {
		return false, 0, err
	}
	if !enabled {
		return false, limit, nil
	}
	if scopePanel.Valid && scopePanel.String != panelID {
		return false, limit, nil
	}
	if scopeInbound.Valid && scopeInbound.Int64 != inboundID {
		return false, limit, nil
	}
	return true, limit, nil
}

func ownedShrinkCandidates(active []sanaei.Client, owned map[string]ownedPolicy, target int, limit int) []string {
	activeCount := 0
	for _, c := range active {
		if c.Enable {
			activeCount++
		}
	}
	excess := activeCount - target
	if excess <= 0 || limit <= 0 {
		return nil
	}
	if excess > limit {
		excess = limit
	}
	ids := make([]string, 0)
	for _, c := range active {
		if !c.Enable {
			continue
		}
		o, ok := owned[c.ID]
		if !ok || c.Email != o.Email || !ownershipMatches(c.Email, o.Marker) {
			continue
		}
		ids = append(ids, c.ID)
	}
	sort.Strings(ids)
	if len(ids) > excess {
		ids = ids[:excess]
	}
	return ids
}

func (s Service) markOwnedDeleted(ctx context.Context, panelID string, inboundID int64, ids []string) error {
	for _, id := range ids {
		if _, err := s.DB.ExecContext(ctx, `
UPDATE bulk_user_ownership o SET state='DELETED',deleted_at=now()
FROM bulk_user_generations g
WHERE o.generation_id=g.id AND g.panel_id=$1 AND g.inbound_id=$2
AND o.client_id=$3 AND o.state IN ('ACTIVE','DELETE_PENDING')
`, panelID, inboundID, id); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) ShrinkDryRun(ctx context.Context, panelID string, inboundID int64, active []sanaei.Client, target, limit int) ([]string, error) {
	owned, err := s.activeOwnedPolicy(ctx, panelID, inboundID)
	if err != nil {
		return nil, err
	}
	return ownedShrinkCandidates(active, owned, target, limit), nil
}

func (s Service) ShrinkDryRunRuntime(ctx context.Context, panelID string, inboundID int64, runtime *sanaei.PanelRuntime, target, limit int) ([]string, error) {
	raws, err := runtime.Session.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	for _, raw := range raws {
		var in rawInbound
		if json.Unmarshal(raw, &in) != nil || int64(in.ID) != inboundID {
			continue
		}
		var st struct {
			Clients []sanaei.Client `json:"clients"`
		}
		if json.Unmarshal(in.Settings, &st) != nil {
			return nil, fmt.Errorf("inbound %d settings", inboundID)
		}
		return s.ShrinkDryRun(ctx, panelID, inboundID, st.Clients, target, limit)
	}
	return nil, fmt.Errorf("inbound %d missing", inboundID)
}
