package usercapacity

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

type rollbackOwned struct {
	ClientID string
	Email    string
}

func (s Service) ownedRollbackBatch(ctx context.Context, generationID string, limit int) ([]rollbackOwned, string, string, int64, error) {
	if limit <= 0 || limit > 32 {
		limit = 32
	}
	var panelID, marker string
	var inboundID int64
	var state string
	err := s.DB.QueryRowContext(ctx,
		"SELECT panel_id::text,marker,inbound_id,state FROM bulk_user_generations WHERE id=$1",
		generationID,
	).Scan(&panelID, &marker, &inboundID, &state)
	if err != nil {
		return nil, "", "", 0, err
	}
	if state == "CLOSED" {
		return nil, panelID, marker, inboundID, nil
	}

	rows, err := s.DB.QueryContext(ctx,
		"SELECT client_id,email FROM bulk_user_ownership WHERE generation_id=$1 AND state IN ('PLANNED','ACTIVE','DELETE_PENDING') ORDER BY created_at LIMIT $2",
		generationID, limit,
	)
	if err != nil {
		return nil, "", "", 0, err
	}
	defer rows.Close()
	out := make([]rollbackOwned, 0, limit)
	for rows.Next() {
		var item rollbackOwned
		if err := rows.Scan(&item.ClientID, &item.Email); err != nil {
			return nil, "", "", 0, err
		}
		out = append(out, item)
	}
	return out, panelID, marker, inboundID, rows.Err()
}

func observedClients(raw json.RawMessage) (map[string]string, error) {
	var in struct {
		Settings any `json:"settings"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	var b []byte
	switch v := in.Settings.(type) {
	case string:
		b = []byte(v)
	case map[string]any:
		var err error
		b, err = json.Marshal(v)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("ownership inbound settings")
	}
	var settings struct {
		Clients []sanaei.Client `json:"clients"`
	}
	if err := json.Unmarshal(b, &settings); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(settings.Clients))
	for _, c := range settings.Clients {
		if c.ID != "" {
			out[c.ID] = c.Email
		}
	}
	return out, nil
}

func (s Service) RollbackOwnedGeneration(ctx context.Context, runtime *sanaei.PanelRuntime, generationID string) (bool, error) {
	items, panelID, marker, inboundID, err := s.ownedRollbackBatch(ctx, generationID, 32)
	if err != nil {
		return false, err
	}
	if runtime == nil || runtime.Session == nil || runtime.PanelID != panelID {
		return false, errors.New("ownership rollback runtime mismatch")
	}
	if len(items) == 0 {
		_, err = s.DB.ExecContext(ctx, "UPDATE bulk_user_generations SET state='CLOSED',closed_at=now() WHERE id=$1 AND state<>'CLOSED'", generationID)
		return false, err
	}
	_, _ = s.DB.ExecContext(ctx, "UPDATE bulk_user_generations SET state='ROLLING_BACK' WHERE id=$1 AND state='ACTIVE'", generationID)
	runtime.Session.Invalidate()
	raw, found, err := runtime.Session.RawInbound(ctx, inboundID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, errors.New("ownership rollback inbound missing")
	}
	observed, err := observedClients(raw)
	if err != nil {
		return false, err
	}

	mutated := false
	for _, item := range items {
		actual, exists := observed[item.ClientID]
		if !exists {
			_, err = s.DB.ExecContext(ctx, "UPDATE bulk_user_ownership SET state='DELETED',deleted_at=now() WHERE generation_id=$1 AND client_id=$2", generationID, item.ClientID)
			if err != nil {
				return mutated, err
			}
			continue
		}
		if actual != item.Email || !ownershipMatches(actual, marker) {
			return mutated, errors.New("ownership rollback marker mismatch")
		}
		if _, err = s.DB.ExecContext(ctx, "UPDATE bulk_user_ownership SET state='DELETE_PENDING' WHERE generation_id=$1 AND client_id=$2 AND state<>'DELETED'", generationID, item.ClientID); err != nil {
			return mutated, err
		}
		if err = sanaei.DeleteClientSession(ctx, runtime.Session.Exec, int(inboundID), item.ClientID); err != nil {
			return mutated, err
		}
		if _, err = s.DB.ExecContext(ctx, "UPDATE bulk_user_ownership SET state='DELETED',deleted_at=now() WHERE generation_id=$1 AND client_id=$2", generationID, item.ClientID); err != nil {
			return true, err
		}
		mutated = true
	}
	runtime.Session.Invalidate()
	return mutated, nil
}
