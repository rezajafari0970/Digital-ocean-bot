package digitalocean

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/resources"
)

type SyncService struct {
	Client    *Client
	Snapshots SnapshotStore
	Registry  resources.Registry
}

type SyncResult struct {
	Snapshot    Snapshot
	Differences []resources.Difference
}

func (s SyncService) Run(ctx context.Context) (SyncResult, error) {
	discovery, err := s.Client.Discover(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	version, err := s.Snapshots.NextVersion(ctx, s.Client.Account.AccountID)
	if err != nil {
		return SyncResult{}, err
	}
	snap := NewSnapshot(s.Client.Account.AccountID, version, discovery)
	if err := s.Snapshots.Save(ctx, snap); err != nil {
		return SyncResult{}, err
	}
	remote := RegistryResources(s.Client.Account.AccountID, discovery)
	local, err := s.Registry.Find(ctx, s.Client.Account.AccountID)
	if err != nil {
		return SyncResult{}, err
	}
	diffs := resources.Compare(local, remote)
	if err := s.Registry.Sync(ctx, s.Client.Account.AccountID, remote); err != nil {
		return SyncResult{}, err
	}
	return SyncResult{Snapshot: snap, Differences: diffs}, nil
}
