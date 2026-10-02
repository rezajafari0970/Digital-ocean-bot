package app

import "context"

func (c Container) runtimeForLifecycle(ctx context.Context, accountID string) (AccountRuntime, error) {
	var enabled, deletionRequested bool
	if err := c.DB.QueryRowContext(ctx, `SELECT enabled,deletion_requested_at IS NOT NULL FROM accounts WHERE id=$1`, accountID).Scan(&enabled, &deletionRequested); err != nil {
		return AccountRuntime{}, err
	}
	if !enabled && deletionRequested {
		return c.CleanupRuntime(ctx, accountID)
	}
	return c.Runtime(ctx, accountID)
}
