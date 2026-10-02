package app

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

func loadDeploymentProfileTx(ctx context.Context, tx *sql.Tx, id string) (workflow.PersistentProfile, error) {
	var p workflow.PersistentProfile
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT id::text,account_id::text,name,version,config,enabled,created_at,updated_at FROM deployment_profiles WHERE id=$1`, id).
		Scan(&p.ID, &p.AccountID, &p.Name, &p.Version, &raw, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(raw, &p.Config)
	return p, err
}
