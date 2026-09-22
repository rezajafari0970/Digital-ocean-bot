package app

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
)

func (c Container) ProcessLifecycle(ctx context.Context, item droplets.LifecycleItem) error {
	runtime, err := c.Runtime(ctx, item.AccountID)
	if err != nil {
		return err
	}
	executor := droplets.Executor{Operations: runtime.Operations, Provider: runtime.Provider, Gate: runtime.Gate}
	engine := droplets.LifecycleEngine{Store: droplets.LifecycleStore{DB: c.DB}, Executor: executor}
	return engine.Process(ctx, item)
}

func (c Container) ConfirmDeleted(ctx context.Context, accountID, providerID string) error {
	_, err := c.DB.ExecContext(ctx, `UPDATE droplets SET state='DELETED',updated_at=now() WHERE account_id=$1 AND provider_resource_id=$2 AND state='DELETING'`, accountID, providerID)
	return err
}
