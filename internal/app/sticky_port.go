package app

import (
	"context"
	"database/sql"
	"errors"
)

var ErrStickyPortUnavailable = errors.New("no sticky proxy port available")

func (c Container) ensureAccountStickyPort(ctx context.Context, accountID string) (int, error) {
	if c.DB == nil {
		return 0, ErrStickyPortUnavailable
	}
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "sticky-port-allocation"); err != nil {
		return 0, err
	}

	var port sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT sticky_port FROM account_network_identities WHERE account_id=$1 FOR UPDATE`, accountID).Scan(&port)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if port.Valid && port.Int64 >= 10000 && port.Int64 <= 20000 {
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		return int(port.Int64), nil
	}

	var next int
	if err := tx.QueryRowContext(ctx, `
SELECT p
FROM generate_series(10000,20000) AS p
WHERE NOT EXISTS (
  SELECT 1 FROM account_network_identities i WHERE i.sticky_port=p
)
ORDER BY p
LIMIT 1
`).Scan(&next); errors.Is(err, sql.ErrNoRows) {
		return 0, ErrStickyPortUnavailable
	} else if err != nil {
		return 0, err
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO account_network_identities(account_id,timezone,locale,sticky_port,updated_at)
VALUES($1,'UTC','en-US',$2,now())
ON CONFLICT(account_id) DO UPDATE SET sticky_port=$2,updated_at=now()
`, accountID, next)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return next, nil
}
