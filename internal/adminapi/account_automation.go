package adminapi

import (
	"context"
	"encoding/json"
)

func (s *Server) syncAccountAutomation(ctx context.Context, accountID string) error {
	var region, image string
	var sizesRaw []byte
	var interval, batch, concurrent, lifetime int
	err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(preferred_region,''),preferred_sizes,COALESCE(preferred_image,''),auto_interval_seconds,auto_batch_size,auto_max_concurrent,server_lifetime_seconds FROM accounts WHERE id=$1`, accountID).Scan(&region, &sizesRaw, &image, &interval, &batch, &concurrent, &lifetime)
	if err != nil {
		return err
	}
	var sizes []string
	_ = json.Unmarshal(sizesRaw, &sizes)
	size := ""
	if len(sizes) > 0 {
		size = sizes[0]
	}

	cfg := map[string]any{
		"name":         "account-auto",
		"region":       region,
		"size":         size,
		"image":        image,
		"lifetime":     lifetime * 1000000000,
		"ssh_user":     "root",
		"inbound_id":   1,
		"client_count": 1,
		"email_prefix": "client",
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	var profileID string
	err = s.DB.QueryRowContext(ctx, `INSERT INTO deployment_profiles(id,account_id,name,version,config,enabled)
VALUES(gen_random_uuid(),$1,'account-auto',1,$2,true)
ON CONFLICT(account_id,name,version) DO UPDATE SET config=EXCLUDED.config,enabled=true,updated_at=now()
RETURNING id::text`, accountID, raw).Scan(&profileID)
	if err != nil {
		return err
	}

	_, err = s.DB.ExecContext(ctx, `INSERT INTO schedules(id,account_id,profile_id,enabled,interval_seconds,batch_size,max_concurrent,next_run_at)
VALUES(gen_random_uuid(),$1,$2,true,$3,$4,$5,now()+($3 * interval '1 second'))
ON CONFLICT(account_id,profile_id) DO UPDATE SET enabled=true,interval_seconds=EXCLUDED.interval_seconds,batch_size=EXCLUDED.batch_size,max_concurrent=EXCLUDED.max_concurrent,next_run_at=LEAST(schedules.next_run_at,now()+($3 * interval '1 second')),updated_at=now()`, accountID, profileID, interval, batch, concurrent)
	return err
}
