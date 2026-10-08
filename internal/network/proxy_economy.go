package network

import (
	"context"
	"database/sql"
	"time"
)

const (
	EconomyIdentityInterval = 120 * time.Second
	EconomyIdentityMaxAge   = 180 * time.Second
	EconomyBaseInterval     = 300 * time.Second
	EconomyBaseMaxAge       = 360 * time.Second
	EconomyCatalogTTL       = 6 * time.Hour
)

// Nil controllers retain existing behavior in standalone tools and fixtures.
// A canary never changes the shared base-proxy monitor for other accounts.
type EconomyController struct{ DB *sql.DB }

func (c *EconomyController) Enabled(ctx context.Context, accountID string) (bool, error) {
	if c == nil {
		return false, nil
	}
	var enabled bool
	err := c.DB.QueryRowContext(ctx, `SELECT enabled AND (canary_account_id IS NULL OR canary_account_id::text=$1) FROM proxy_economy_policy WHERE singleton`, accountID).Scan(&enabled)
	return enabled, err
}
