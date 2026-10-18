// Package configauthority selects the single owner of configuration creation.
package configauthority

import (
	"context"
	"database/sql"
)

type Queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func Unified(ctx context.Context, db Queryer) (bool, error) {
	var v bool
	e := db.QueryRowContext(ctx, "SELECT unified FROM config_control_authority WHERE singleton").Scan(&v)
	return v, e
}

// SQL fences are evaluated in the same snapshot as the protected operation.
const Legacy = ` NOT EXISTS(SELECT 1 FROM config_control_authority WHERE unified) `
