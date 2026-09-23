package app

import "context"

type ScheduledStarter struct{ Container Container }

func (s ScheduledStarter) StartScheduledDeployment(ctx context.Context, accountID, profileID string) error {
	_, err := s.Container.StartDeployment(ctx, accountID, profileID)
	return err
}
