package app

import "context"

type ScheduledStarter struct{ Container Container }

func (s ScheduledStarter) PrepareScheduledAccount(ctx context.Context, accountID string) error {
	// Scheduled readiness checks share the keeper lease/cadence. The actual
	// deployment path still performs its fresh identity and mutation checks.
	if enabled, err := s.Container.Economy.Enabled(ctx, accountID); err != nil {
		return err
	} else if enabled {
		var eligible bool
		if err := s.Container.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1 AND deleted_at IS NULL AND (enabled OR deletion_requested_at IS NOT NULL))`, accountID).Scan(&eligible); err != nil {
			return err
		}
		if !eligible {
			return ErrAccountDisabled
		}
		return s.Container.MaintainProxyControlPlane(ctx, accountID)
	}
	return s.Container.MaintainStickyIdentity(ctx, accountID)
}

func (s ScheduledStarter) StartScheduledDeployment(ctx context.Context, accountID, profileID string, consumeBackfill bool) error {
	_, err := s.Container.prepareDeployment(ctx, accountID, profileID, consumeBackfill, "", true)
	return err
}
