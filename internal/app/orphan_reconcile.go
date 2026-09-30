package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"strings"
	"time"
)

// ReconcileOwnedOrphans only acts on provider droplets carrying BOTH bot ownership
// and a deployment-specific tag whose deployment exists for the same account.
// It never touches untagged/foreign resources. A locally DELETED but provider-live
// owned droplet is restored to RETIRING so the normal idempotent lifecycle deletes it.
func (c Container) ReconcileOwnedOrphans(ctx context.Context, accountID string) {
	var raw []byte
	var at time.Time
	if err := c.DB.QueryRowContext(ctx, `SELECT canonical,created_at FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, accountID).Scan(&raw, &at); err != nil || time.Since(at) > 2*time.Minute {
		return
	}
	var snap providers.Observation
	if json.Unmarshal(raw, &snap) != nil {
		return
	}
	for _, x := range snap.Inventory.Servers {
		owned := false
		depID := ""
		for _, t := range x.Tags {
			if t == "managed-by-digital-ocean-bot" {
				owned = true
			}
			if strings.HasPrefix(t, "dob-deployment-") {
				depID = strings.TrimPrefix(t, "dob-deployment-")
			}
		}
		if !owned || depID == "" {
			continue
		}
		var localID, state string
		err := c.DB.QueryRowContext(ctx, `SELECT dr.id::text,dr.state FROM deployments d JOIN droplets dr ON dr.id=d.droplet_id WHERE d.id=$1::uuid AND d.account_id=$2 AND dr.provider_resource_id=$3`, depID, accountID, x.ID).Scan(&localID, &state)
		var depState string
		if c.DB.QueryRowContext(ctx, `SELECT state FROM deployments WHERE id=$1::uuid AND account_id=$2`, depID, accountID).Scan(&depState) != nil || depState != "FAILED" {
			continue
		}
		if errors.Is(err, sql.ErrNoRows) {
			err = c.DB.QueryRowContext(ctx, `INSERT INTO droplets(id,account_id,profile_id,provider_resource_id,state,profile) SELECT gen_random_uuid(),d.account_id,d.profile_id,$3,'RETIRING','{}'::jsonb FROM deployments d WHERE d.id=$1::uuid AND d.account_id=$2 AND d.state='FAILED' AND NOT EXISTS(SELECT 1 FROM droplets z WHERE z.account_id=$2 AND z.provider_resource_id=$3 AND z.state<>'DELETED') RETURNING id::text`, depID, accountID, x.ID).Scan(&localID)
			if err == nil {
				_, _ = c.DB.ExecContext(ctx, `INSERT INTO lifecycle_events(id,account_id,resource_id,state) VALUES(gen_random_uuid(),$1,$2::uuid,'RETIRING')`, accountID, localID)
			}
			continue
		}
		if err != nil || state != "DELETED" {
			continue
		}
		res, err := c.DB.ExecContext(ctx, `UPDATE droplets SET state='RETIRING',replacement_deployment_id=NULL,updated_at=now() WHERE id=$1::uuid AND account_id=$2 AND provider_resource_id=$3 AND state='DELETED'`, localID, accountID, x.ID)
		if err != nil {
			continue
		}
		n, _ := res.RowsAffected()
		if n == 1 {
			_, _ = c.DB.ExecContext(ctx, `INSERT INTO lifecycle_events(id,account_id,resource_id,state) VALUES(gen_random_uuid(),$1,$2::uuid,'RETIRING')`, accountID, localID)
		}
	}
}
