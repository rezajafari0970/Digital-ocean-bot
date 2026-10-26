package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"time"
)

// SyncCloudResources accepts only a successful complete inventory. Management
// requires both local ownership and this account's tags; foreign resources stay read-only.
func (c Container) SyncCloudResources(ctx context.Context, accountID, provider string, inv providers.Inventory) error {
	tx, e := c.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = syncCloudResourcesTx(ctx, tx, accountID, provider, inv); e != nil {
		return e
	}
	return tx.Commit()
}
func syncCloudResourcesTx(ctx context.Context, tx *sql.Tx, accountID, provider string, inv providers.Inventory) error {
	var e error
	if provider != "aws" && provider != "ovh" && provider != "kamatera" {
		return fmt.Errorf("unsupported cloud registry provider")
	}

	if inv.Servers == nil || inv.ObservedAt.IsZero() || inv.ObservedAt.After(time.Now().Add(time.Minute)) || time.Since(inv.ObservedAt) > 2*time.Minute {
		return fmt.Errorf("Cloud inventory is missing or stale")
	}
	seen := map[string]bool{}
	for _, s := range inv.Servers {
		if s.ID == "" || seen[s.ID] {
			return fmt.Errorf("Cloud inventory identity invalid")
		}
		seen[s.ID] = true
	}

	ids := []string{}
	for _, s := range inv.Servers {
		var local bool
		if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM droplets WHERE account_id=$1 AND provider_resource_id=$2 AND state<>'DELETED')", accountID, s.ID).Scan(&local); e != nil {
			return e
		}
		owner, managed := false, false
		for _, tag := range s.Tags {
			owner = owner || tag == "dob-account-"+accountID
			managed = managed || tag == "managed-by-digital-ocean-bot"
		}
		meta, e := json.Marshal(map[string]any{"region": s.RegionID, "name": s.Name, "ready": s.Ready, "ipv4": s.PrimaryIPv4})
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed,metadata,created_at,updated_at)
 VALUES(gen_random_uuid(),$1,$6,$2,'server',$3,$4,$5,now(),now())
 ON CONFLICT(account_id,provider,type,provider_resource_id) DO UPDATE SET state=EXCLUDED.state,managed=EXCLUDED.managed,metadata=EXCLUDED.metadata,updated_at=now()`, accountID, s.ID, string(s.State), local && owner && managed, meta, provider); e != nil {
			return e
		}
		ids = append(ids, s.ID)
	}
	if _, e = tx.ExecContext(ctx, "UPDATE resources SET state='deleted',managed=false,updated_at=now() WHERE account_id=$1 AND provider=$3 AND type='server' AND state<>'deleted' AND NOT(provider_resource_id=ANY($2))", accountID, pq.Array(ids), provider); e != nil {
		return e
	}
	return nil
}
