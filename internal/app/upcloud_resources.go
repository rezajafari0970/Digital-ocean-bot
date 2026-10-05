package app

import (
	"context"
	"encoding/json"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

// Called only after a complete, successful provider inventory.
func (c Container) SyncUpCloudResources(ctx context.Context, accountID string, inv providers.Inventory) error {
	tx, e := c.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	serverIDs := []string{}
	storageIDs := []string{}
	put := func(id, kind, state string, managed bool, meta map[string]any) error {
		raw, e := json.Marshal(meta)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed,metadata,created_at,updated_at)
   VALUES(gen_random_uuid(),$1,'upcloud',$2,$3,$4,$5,$6,now(),now())
   ON CONFLICT(account_id,provider,type,provider_resource_id) DO UPDATE SET state=EXCLUDED.state,managed=EXCLUDED.managed,metadata=EXCLUDED.metadata,updated_at=now()`, accountID, id, kind, state, managed, raw)
		return e
	}
	for _, s := range inv.Servers {
		var managed bool
		if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM droplets WHERE account_id=$1 AND provider_resource_id=$2 AND state<>'DELETED')", accountID, s.ID).Scan(&managed); e != nil {
			return e
		}
		meta := map[string]any{"region": s.RegionID, "name": s.Name, "ready": s.Ready, "ipv4": s.PrimaryIPv4}
		if e = put(s.ID, "server", string(s.State), managed, meta); e != nil {
			return e
		}
		serverIDs = append(serverIDs, s.ID)
	}
	for _, s := range inv.Resources {
		if s.Type == "storage" {
			if e = put(s.ID, "storage", s.State, s.Managed, s.Metadata); e != nil {
				return e
			}
			storageIDs = append(storageIDs, s.ID)
		}
	}
	for kind, ids := range map[string][]string{"server": serverIDs, "storage": storageIDs} {
		if _, e = tx.ExecContext(ctx, "UPDATE resources SET state='deleted',managed=false,updated_at=now() WHERE account_id=$1 AND provider='upcloud' AND type=$2 AND state<>'deleted' AND NOT(provider_resource_id=ANY($3))", accountID, kind, pq.Array(ids)); e != nil {
			return e
		}
	}
	return tx.Commit()
}
