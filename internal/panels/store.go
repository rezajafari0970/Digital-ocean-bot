package panels

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type SQLStore struct {
	DB *sql.DB
}

func (s SQLStore) SaveDiscovery(
	ctx context.Context,
	panelID string,
	d Discovery,
) error {

	capabilities, err := json.Marshal(d.Capabilities)
	if err != nil {
		return err
	}

	evidence, err := json.Marshal(d.Evidence)
	if err != nil {
		return err
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	_, err = tx.ExecContext(
		ctx,
		`
UPDATE panel_instances
SET
version=$2,
last_seen_at=$3,
updated_at=now()
WHERE id=$1
`,
		panelID,
		d.Version,
		d.ObservedAt,
	)

	if err != nil {
		return err
	}

	_, err = tx.ExecContext(
		ctx,
		`
INSERT INTO panel_capability_snapshots(
id,
panel_id,
driver,
version,
capabilities,
evidence,
observed_at
)
VALUES(
gen_random_uuid(),
$1,
$2,
$3,
$4,
$5,
$6
)
`,
		panelID,
		d.Driver,
		d.Version,
		capabilities,
		evidence,
		d.ObservedAt,
	)

	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s SQLStore) ReconcileEnabled(
	ctx context.Context,
) (int64, error) {

	if s.DB == nil {
		return 0,
			errors.New(
				"panel store unavailable",
			)
	}

	result, err := s.DB.ExecContext(
		ctx,
		`
UPDATE panel_instances pi

SET
enabled=false,
updated_at=now()

FROM droplets d

WHERE d.id=pi.droplet_id

  AND pi.enabled=true

  AND d.state='DELETED'
`,
	)

	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

type InstanceReconcileResult struct {
	ReadyUpserts    int64
	DeletedDisabled int64
}

func (s SQLStore) ReconcileInstances(ctx context.Context) (InstanceReconcileResult, error) {
	if s.DB == nil {
		return InstanceReconcileResult{}, errors.New("panel store unavailable")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return InstanceReconcileResult{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
INSERT INTO panel_instances(account_id,droplet_id,driver,base_url,auth_secret_ref,version,enabled,updated_at)
SELECT d.account_id,d.droplet_id,'sanaei-3x-ui',
       'http://' || d.host || ':' || x.port::text || x.web_path,
       x.password_secret_ref,'',true,now()
FROM deployments d
JOIN droplets r ON r.id=d.droplet_id
JOIN xui_panel_deployments x ON x.droplet_id=d.droplet_id AND x.generation=d.postinstall_generation
WHERE d.state='PANEL_COMPLETE' AND r.state='READY' AND x.state='COMPLETED'
ON CONFLICT(droplet_id,driver) DO UPDATE SET
 account_id=excluded.account_id,
 base_url=excluded.base_url,
 auth_secret_ref=excluded.auth_secret_ref,
 enabled=true,
 updated_at=now()
WHERE panel_instances.account_id IS DISTINCT FROM excluded.account_id
   OR panel_instances.base_url IS DISTINCT FROM excluded.base_url
   OR panel_instances.auth_secret_ref IS DISTINCT FROM excluded.auth_secret_ref
   OR panel_instances.enabled IS DISTINCT FROM true
`)
	if err != nil {
		return InstanceReconcileResult{}, err
	}
	up, err := res.RowsAffected()
	if err != nil {
		return InstanceReconcileResult{}, err
	}
	res, err = tx.ExecContext(ctx, `
UPDATE panel_instances pi SET enabled=false,updated_at=now()
FROM droplets d
WHERE d.id=pi.droplet_id AND pi.enabled=true AND d.state='DELETED'
`)
	if err != nil {
		return InstanceReconcileResult{}, err
	}
	down, err := res.RowsAffected()
	if err != nil {
		return InstanceReconcileResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return InstanceReconcileResult{}, err
	}
	return InstanceReconcileResult{ReadyUpserts: up, DeletedDisabled: down}, nil
}
