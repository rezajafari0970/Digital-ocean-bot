package digitalocean

import (
	"context"
	"database/sql"
	"encoding/json"
)

type SnapshotStore struct{ DB *sql.DB }

func (s SnapshotStore) Save(ctx context.Context, snap Snapshot) error {
	data,err:=json.Marshal(snap.Data); if err!=nil{return err}
	_,err=s.DB.ExecContext(ctx,`INSERT INTO provider_snapshots(id,account_id,provider,version,data,created_at) VALUES(gen_random_uuid(),$1,$2,$3,$4,$5)`,snap.AccountID,snap.Provider,snap.Version,data,snap.CreatedAt)
	return err
}

func (s SnapshotStore) NextVersion(ctx context.Context, accountID string)(int64,error){
	var version int64
	err:=s.DB.QueryRowContext(ctx,`SELECT COALESCE(MAX(version),0)+1 FROM provider_snapshots WHERE account_id=$1 AND provider='digitalocean'`,accountID).Scan(&version)
	return version,err
}
