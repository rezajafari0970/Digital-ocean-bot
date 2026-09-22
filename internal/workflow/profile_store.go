package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
)

type ProfileStore struct{ DB *sql.DB }

func (s ProfileStore) Get(ctx context.Context, id string) (PersistentProfile, error) {
	var p PersistentProfile
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `SELECT id::text,account_id::text,name,version,config,enabled,created_at,updated_at FROM deployment_profiles WHERE id=$1`, id).Scan(&p.ID, &p.AccountID, &p.Name, &p.Version, &raw, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(raw, &p.Config)
	return p, err
}

func (s ProfileStore) SnapshotForDeployment(ctx context.Context, deploymentID string) (ProfileSnapshot, error) {
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `SELECT profile_snapshot FROM deployments WHERE id=$1`, deploymentID).Scan(&raw)
	if err != nil {
		return ProfileSnapshot{}, err
	}
	var p ProfileSnapshot
	err = json.Unmarshal(raw, &p)
	return p, err
}

func (s ProfileStore) AttachSnapshot(ctx context.Context, deploymentID string, p ProfileSnapshot) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE deployments SET profile_snapshot=$2 WHERE id=$1 AND profile_snapshot='{}'::jsonb`, deploymentID, raw)
	return err
}
