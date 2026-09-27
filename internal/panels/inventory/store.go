package inventory

import (
	"context"
	"database/sql"
	"errors"
)

var ErrInvalidSnapshot = errors.New("invalid inventory snapshot")

type SyncResult struct {
	Observed int
	Changed  int
	Missing  int
}

type SQLStore struct {
	DB *sql.DB
}

func (s SQLStore) Sync(
	ctx context.Context,
	snapshot Snapshot,
) (SyncResult, error) {

	if s.DB == nil ||
		snapshot.PanelID == "" ||
		snapshot.ObservedAt.IsZero() {

		return SyncResult{},
			ErrInvalidSnapshot
	}

	tx, err := s.DB.BeginTx(
		ctx,
		nil,
	)

	if err != nil {
		return SyncResult{}, err
	}

	defer tx.Rollback()

	var syncID string

	err = tx.QueryRowContext(
		ctx,
		`
INSERT INTO panel_inventory_syncs(
id,
panel_id,
state,
started_at
)
VALUES(
gen_random_uuid(),
$1,
'RUNNING',
$2
)
RETURNING id::text
`,
		snapshot.PanelID,
		snapshot.ObservedAt,
	).Scan(
		&syncID,
	)

	if err != nil {
		return SyncResult{}, err
	}

	// Mark the previous current-state as unseen.
	//
	// Because this happens inside the same transaction,
	// any later failure rolls this change back.
	_, err = tx.ExecContext(
		ctx,
		`
UPDATE panel_inbound_inventory
SET
present=false,
missing_since=COALESCE(
missing_since,
$2
)
WHERE panel_id=$1
  AND present=true
`,
		snapshot.PanelID,
		snapshot.ObservedAt,
	)

	if err != nil {
		return SyncResult{}, err
	}

	changed := 0

	for _, record := range snapshot.Records {

		if record.PanelID != "" &&
			record.PanelID != snapshot.PanelID {

			return SyncResult{},
				ErrInvalidSnapshot
		}

		var previousHash sql.NullString

		err = tx.QueryRowContext(
			ctx,
			`
SELECT raw_hash
FROM panel_inbound_inventory
WHERE panel_id=$1
  AND remote_id=$2
`,
			snapshot.PanelID,
			record.RemoteID,
		).Scan(
			&previousHash,
		)

		switch {
		case errors.Is(
			err,
			sql.ErrNoRows,
		):
			changed++

		case err != nil:
			return SyncResult{}, err

		case !previousHash.Valid:
			changed++

		case previousHash.String !=
			record.RawHash:
			changed++
		}

		_, err = tx.ExecContext(
			ctx,
			`
INSERT INTO panel_inbound_inventory(
panel_id,
remote_id,

remark,
protocol,

port,
listen,

enabled,

transport,
security,

client_count,

upload_bytes,
download_bytes,
total_bytes,

raw_hash,

present,

first_seen_at,
last_seen_at,

missing_since
)
VALUES(
$1,
$2,

$3,
$4,

$5,
$6,

$7,

$8,
$9,

$10,

$11,
$12,
$13,

$14,

true,

$15,
$15,

NULL
)

ON CONFLICT(
panel_id,
remote_id
)

DO UPDATE SET
remark=excluded.remark,

protocol=excluded.protocol,

port=excluded.port,

listen=excluded.listen,

enabled=excluded.enabled,

transport=excluded.transport,

security=excluded.security,

client_count=excluded.client_count,

upload_bytes=excluded.upload_bytes,

download_bytes=excluded.download_bytes,

total_bytes=excluded.total_bytes,

raw_hash=excluded.raw_hash,

present=true,

last_seen_at=excluded.last_seen_at,

missing_since=NULL
`,
			snapshot.PanelID,
			record.RemoteID,

			record.Remark,
			record.Protocol,

			record.Port,
			record.Listen,

			record.Enabled,

			record.Transport,
			record.Security,

			record.ClientCount,

			record.Upload,
			record.Download,
			record.Total,

			record.RawHash,

			snapshot.ObservedAt,
		)

		if err != nil {
			return SyncResult{}, err
		}
	}

	var missing int

	err = tx.QueryRowContext(
		ctx,
		`
SELECT COUNT(*)
FROM panel_inbound_inventory
WHERE panel_id=$1
  AND present=false
  AND missing_since=$2
`,
		snapshot.PanelID,
		snapshot.ObservedAt,
	).Scan(
		&missing,
	)

	if err != nil {
		return SyncResult{}, err
	}

	_, err = tx.ExecContext(
		ctx,
		`
UPDATE panel_inventory_syncs
SET
state='COMPLETED',

observed_count=$2,

changed_count=$3,

missing_count=$4,

finished_at=now()

WHERE id=$1
`,
		syncID,

		len(snapshot.Records),

		changed,

		missing,
	)

	if err != nil {
		return SyncResult{}, err
	}

	if err = tx.Commit(); err != nil {
		return SyncResult{}, err
	}

	return SyncResult{
		Observed: len(snapshot.Records),

		Changed: changed,

		Missing: missing,
	}, nil
}
