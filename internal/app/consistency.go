package app

import (
	"context"
	"encoding/json"
	"log"
)

// ReconcileLocalState repairs only relationships that are provable from local
// persisted state. It never creates or deletes provider resources.
func (c Container) ReconcileLocalState(ctx context.Context) {
	// Link deployments to an already-known lifecycle droplet when provider IDs agree.
	_, _ = c.DB.ExecContext(ctx, `UPDATE deployments d SET droplet_id=dr.id,updated_at=now()
        FROM droplets dr WHERE d.droplet_id IS NULL AND d.provider_id IS NOT NULL
        AND dr.account_id=d.account_id AND dr.provider_resource_id=d.provider_id AND dr.state<>'DELETED'`)

	// Mirror lifecycle droplets into inventory. This is an upsert and therefore
	// safe to run repeatedly.
	rows, err := c.DB.QueryContext(ctx, `SELECT id::text,account_id::text,provider_resource_id,state,profile FROM droplets WHERE provider_resource_id IS NOT NULL AND state<>'DELETED'`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var did, aid, pid, state string
		var profile []byte
		if rows.Scan(&did, &aid, &pid, &state, &profile) != nil {
			continue
		}
		meta, _ := json.Marshal(map[string]any{"droplet_id": did, "profile": json.RawMessage(profile)})
		resourceState := "provisioning"
		if state == "READY" || state == "EXPIRING" || state == "RETIRING" {
			resourceState = "active"
		}
		_, err = c.DB.ExecContext(ctx, `INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed,metadata)
            VALUES(gen_random_uuid(),$1,'digitalocean',$2,'droplet',$3,true,$4)
            ON CONFLICT(account_id,provider,type,provider_resource_id)
            DO UPDATE SET state=EXCLUDED.state,managed=true,metadata=EXCLUDED.metadata,updated_at=now()`, aid, pid, resourceState, meta)
		if err != nil {
			log.Printf("consistency inventory %s: %v", pid, err)
		}
	}
}
