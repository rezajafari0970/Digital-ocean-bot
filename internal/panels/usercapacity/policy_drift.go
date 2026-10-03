package usercapacity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

type ownedPolicy struct {
	Email     string
	Marker    string
	CreatedAt time.Time
}

func (s Service) activeOwnedPolicy(ctx context.Context, panelID string, inboundID int64) (map[string]ownedPolicy, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT o.client_id,o.email,g.marker,o.created_at
FROM bulk_user_ownership o
JOIN bulk_user_generations g ON g.id=o.generation_id
WHERE g.panel_id=$1 AND g.inbound_id=$2 AND o.state='ACTIVE'
`, panelID, inboundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ownedPolicy{}
	for rows.Next() {
		var id string
		var p ownedPolicy
		if err := rows.Scan(&id, &p.Email, &p.Marker, &p.CreatedAt); err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, rows.Err()
}

func desiredOwnedClient(c sanaei.Client, own ownedPolicy, quota int64, life, limit int) (sanaei.Client, bool) {
	if c.Email != own.Email || !ownershipMatches(c.Email, own.Marker) {
		return c, false
	}
	expiry := int64(0)
	if life > 0 {
		expiry = own.CreatedAt.Add(time.Duration(life) * time.Second).UnixMilli()
	}
	changed := c.TotalGB != quota || c.ExpiryTime != expiry || c.LimitIP != limit
	c.TotalGB, c.ExpiryTime, c.LimitIP = quota, expiry, limit
	return c, changed
}

func verifyOwnedPolicySnapshot(raws []json.RawMessage, inboundID int, expected map[string]sanaei.Client, absent []string) error {
	for _, raw := range raws {
		var in rawInbound
		if json.Unmarshal(raw, &in) != nil || in.ID != inboundID {
			continue
		}
		var st struct {
			Clients []sanaei.Client `json:"clients"`
		}
		if json.Unmarshal(in.Settings, &st) != nil {
			return fmt.Errorf("inbound %d policy verify settings", inboundID)
		}
		seen := map[string]sanaei.Client{}
		for _, c := range st.Clients {
			seen[c.ID] = c
		}
		for id, want := range expected {
			got, ok := seen[id]
			if !ok || got.Email != want.Email || got.TotalGB != want.TotalGB || got.ExpiryTime != want.ExpiryTime || got.LimitIP != want.LimitIP {
				return fmt.Errorf("inbound %d client %s policy unconfirmed", inboundID, id)
			}
		}
		for _, id := range absent {
			if _, ok := seen[id]; ok {
				return fmt.Errorf("inbound %d client %s deletion unconfirmed", inboundID, id)
			}
		}
		return nil
	}
	return fmt.Errorf("inbound %d policy verify missing", inboundID)
}
