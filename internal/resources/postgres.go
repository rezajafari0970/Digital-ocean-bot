package resources

import (
	"context"
	"database/sql"
	"encoding/json"
)

type SQLRegistry struct{ DB *sql.DB }

func (r SQLRegistry) Sync(ctx context.Context, accountID string, items []Resource) error {
	tx,err:=r.DB.BeginTx(ctx,nil); if err!=nil{return err}; defer tx.Rollback()
	for _,item:=range items {
		if item.AccountID!=accountID{return ErrTenantMismatch}
		metadata,_:=json.Marshal(item.Metadata)
		_,err=tx.ExecContext(ctx,`INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed,metadata,created_at,updated_at) VALUES(gen_random_uuid(),$1,$2,$3,$4,$5,$6,$7,now(),now()) ON CONFLICT(account_id,provider,type,provider_resource_id) DO UPDATE SET state=EXCLUDED.state,metadata=EXCLUDED.metadata,updated_at=now()`,item.AccountID,item.Provider,item.ProviderResourceID,item.Type,item.State,item.Managed,metadata)
		if err!=nil{return err}
	}
	return tx.Commit()
}

func (r SQLRegistry) Find(ctx context.Context, accountID string)([]Resource,error){
	rows,err:=r.DB.QueryContext(ctx,`SELECT id::text,account_id::text,provider,provider_resource_id,type,state,managed,metadata,created_at,updated_at FROM resources WHERE account_id=$1`,accountID); if err!=nil{return nil,err}; defer rows.Close()
	var out []Resource
	for rows.Next(){ var x Resource; var raw []byte; if err:=rows.Scan(&x.ID,&x.AccountID,&x.Provider,&x.ProviderResourceID,&x.Type,&x.State,&x.Managed,&raw,&x.CreatedAt,&x.UpdatedAt);err!=nil{return nil,err}; _=json.Unmarshal(raw,&x.Metadata); out=append(out,x) }
	return out,rows.Err()
}
