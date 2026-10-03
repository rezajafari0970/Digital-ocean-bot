package usercapacity

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"time"
)

// An immutable v3 batch is never rewritten through the legacy full-inbound
// policy path. Count observed users and leave mutations to the durable journal.
func (s Service) observeBulkCapacity(ctx context.Context, panelID string, in rawInbound) error {
	b := in.Settings
	var encoded string
	if json.Unmarshal(b, &encoded) == nil {
		b = []byte(encoded)
	}
	var settings struct {
		Clients []sanaei.Client `json:"clients"`
	}
	if err := json.Unmarshal(b, &settings); err != nil {
		return err
	}
	var target int
	if err := s.DB.QueryRowContext(ctx, `SELECT target_users_per_inbound FROM global_config_policies WHERE policy_key='reality'`).Scan(&target); err != nil {
		return err
	}
	target, _, err := s.effectiveTargetRate(ctx, panelID, int64(in.ID), target, 0)
	if err != nil {
		return err
	}
	traffic := map[string]sanaei.ClientTraffic{}
	for _, raw := range in.ClientStats {
		var st sanaei.ClientTraffic
		if json.Unmarshal(raw, &st) == nil {
			traffic[st.Email] = st
		}
	}
	active := 0
	for _, c := range settings.Clients {
		if c.Enable && (c.ExpiryTime == 0 || c.ExpiryTime > time.Now().UnixMilli()) && (c.TotalGB == 0 || traffic[c.Email].Up+traffic[c.Email].Down < c.TotalGB) {
			active++
		}
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO user_capacity_snapshots(panel_id,inbound_id,port,target_users,active_users,deficit,last_error,observed_at) VALUES($1,$2,$3,$4,$5,$6,'',now()) ON CONFLICT(panel_id,inbound_id) DO UPDATE SET target_users=excluded.target_users,active_users=excluded.active_users,deficit=excluded.deficit,last_error='',observed_at=now()`, panelID, in.ID, in.Port, target, active, maxInt(target-active, 0))
	if err != nil {
		return fmt.Errorf("bulk capacity observation: %w", err)
	}
	return nil
}
