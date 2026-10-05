package providers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
)

type CleanupManifest struct {
	Identity   string   `json:"identity"`
	StorageIDs []string `json:"storage_ids"`
	Complete   bool     `json:"complete"`
}
type CleanupJournal interface {
	Load(context.Context, string, string) (*CleanupManifest, error)
	Save(context.Context, string, string, CleanupManifest) error
	Finish(context.Context, string, string) error
}
type SQLCleanupJournal struct{ DB *sql.DB }

func (j SQLCleanupJournal) Load(ctx context.Context, accountID, serverID string) (*CleanupManifest, error) {
	var m CleanupManifest
	var raw []byte
	e := j.DB.QueryRowContext(ctx, "SELECT identity,storage_ids,completed_at IS NOT NULL FROM provider_cleanup_manifests WHERE account_id=$1 AND provider='upcloud' AND server_id=$2", accountID, serverID).Scan(&m.Identity, &raw, &m.Complete)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(raw, &m.StorageIDs); e != nil {
		return nil, e
	}
	return &m, nil
}
func (j SQLCleanupJournal) Save(ctx context.Context, accountID, serverID string, m CleanupManifest) error {
	if m.Identity == "" || m.Complete {
		return errors.New("invalid cleanup manifest")
	}
	m.StorageIDs = append([]string{}, m.StorageIDs...)
	sort.Strings(m.StorageIDs)
	raw, e := json.Marshal(m.StorageIDs)
	if e != nil {
		return e
	}
	_, e = j.DB.ExecContext(ctx, "INSERT INTO provider_cleanup_manifests(account_id,provider,server_id,identity,storage_ids) VALUES($1,'upcloud',$2,$3,$4) ON CONFLICT DO NOTHING", accountID, serverID, m.Identity, raw)
	if e != nil {
		return e
	}
	stored, e := j.Load(ctx, accountID, serverID)
	if e != nil {
		return e
	}
	if stored == nil || stored.Identity != m.Identity || !reflect.DeepEqual(stored.StorageIDs, m.StorageIDs) {
		return errors.New("cleanup manifest identity conflict")
	}
	return nil
}
func (j SQLCleanupJournal) Finish(ctx context.Context, accountID, serverID string) error {
	tx, e := j.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, "UPDATE provider_cleanup_manifests SET completed_at=COALESCE(completed_at,now()) WHERE account_id=$1 AND provider='upcloud' AND server_id=$2", accountID, serverID)
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return errors.New("cleanup manifest missing")
	}
	if _, e = tx.ExecContext(ctx, `UPDATE resources SET state='deleted',managed=false,updated_at=now() WHERE account_id=$1 AND provider='upcloud' AND type='storage' AND provider_resource_id IN (SELECT jsonb_array_elements_text(storage_ids) FROM provider_cleanup_manifests WHERE account_id=$1 AND provider='upcloud' AND server_id=$2)`, accountID, serverID); e != nil {
		return e
	}
	return tx.Commit()
}
