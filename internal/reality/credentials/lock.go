package credentials

import (
	"context"
	"database/sql"
)

func credentialLock(
	ctx context.Context,
	tx *sql.Tx,
	panelID string,
	managedKey string,
) error {

	// Two 32-bit hash components are used as
	// the PostgreSQL advisory-lock namespace.
	//
	// The lock lives only for the transaction.
	_, err := tx.ExecContext(
		ctx,
		`
SELECT pg_advisory_xact_lock(
hashtext($1),
hashtext($2)
)
`,
		panelID,
		managedKey,
	)

	return err
}
