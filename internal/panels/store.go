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
