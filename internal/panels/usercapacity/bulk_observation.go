package usercapacity

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configprofiles"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"time"
)

// An immutable v3 batch is never rewritten through the legacy full-inbound
// policy path. Count observed users and leave mutations to the durable journal.
type observationDB interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s Service) observeBulkCapacity(ctx context.Context, panelID string, in rawInbound) error {
	return observeBulkCapacity(ctx, s.DB, panelID, in)
}
func observeBulkCapacity(ctx context.Context, db observationDB, panelID string, in rawInbound) error {
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
	profiles, err := configprofiles.Read(ctx, db, false)
	if err != nil {
		return err
	}
	// Match the durable lifecycle writer's current ownership generation.
	rows, err := db.QueryContext(ctx, `SELECT o.client_id,COALESCE(NULLIF(o.route_class,''),NULLIF(r.route_class,''),'RESIDENTIAL')
 FROM bulk_lifecycle_scopes sc JOIN bulk_user_ownership o ON o.generation_id=sc.generation_id
 LEFT JOIN panel_client_routes r ON r.panel_id=sc.panel_id AND r.client_id=o.client_id
 WHERE sc.panel_id=$1 AND sc.inbound_id=$2 AND o.state='ACTIVE'`, panelID, in.ID)
	if err != nil {
		return err
	}
	managed, classes := map[string]bool{}, map[string]string{}
	for rows.Next() {
		var id, cls string
		if err = rows.Scan(&id, &cls); err != nil {
			rows.Close()
			return err
		}
		managed[id] = true
		classes[id] = cls
	}
	err = rows.Err()
	rows.Close()
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
	clients := map[string]clientops.LifecycleClient{}
	for _, c := range settings.Clients {
		if _, exists := clients[c.ID]; exists || c.ID == "" {
			return fmt.Errorf("duplicate or missing capacity client identity")
		}
		v := clientops.LifecycleClient{Client: c}
		if t, ok := traffic[c.Email]; ok {
			v.Traffic = &t
		}
		clients[c.ID] = v
	}
	counts, err := clientops.ObserveCapacity(profiles, in.Port, clients, managed, classes, time.Now())
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO user_capacity_snapshots(panel_id,inbound_id,port,target_users,active_users,deficit,expired_users,quota_exhausted_users,last_error,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'',now()) ON CONFLICT(panel_id,inbound_id) DO UPDATE SET port=excluded.port,target_users=excluded.target_users,active_users=excluded.active_users,deficit=excluded.deficit,expired_users=excluded.expired_users,quota_exhausted_users=excluded.quota_exhausted_users,created_last_cycle=0,deleted_last_cycle=0,last_error='',observed_at=now()`, panelID, in.ID, in.Port, counts.Target, counts.Active, counts.Deficit, counts.Expired, counts.Quota)
	if err != nil {
		return fmt.Errorf("bulk capacity observation: %w", err)
	}
	return nil
}
