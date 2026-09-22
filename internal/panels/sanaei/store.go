package sanaei

import (
	"context"
	"database/sql"
)

type SQLClientStore struct{ DB *sql.DB }

func (s SQLClientStore) Save(ctx context.Context, r ClientRecord) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO xui_clients(id,account_id,droplet_id,inbound_id,email,enabled,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$7) ON CONFLICT(id) DO UPDATE SET inbound_id=EXCLUDED.inbound_id,email=EXCLUDED.email,enabled=EXCLUDED.enabled,updated_at=now()`, r.ID, r.AccountID, r.DropletID, r.InboundID, r.Email, r.Enabled, r.CreatedAt)
	return err
}

func (s SQLClientStore) SetEnabled(ctx context.Context, accountID, id string, enabled bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE xui_clients SET enabled=$3,updated_at=now() WHERE account_id=$1 AND id=$2`, accountID, id, enabled)
	return err
}
