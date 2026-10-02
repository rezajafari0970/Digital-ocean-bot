package app

import "context"

type ScheduledStarter struct{ Container Container }

func (s ScheduledStarter) PrepareScheduledAccount(ctx context.Context, accountID string) error {
	return s.Container.MaintainStickyIdentity(ctx, accountID)
}

func (s ScheduledStarter) StartScheduledDeployment(ctx context.Context, accountID, profileID string, consumeBackfill bool) error {
	_, err := s.Container.startDeployment(ctx, accountID, profileID, consumeBackfill, "")
	return err
}
