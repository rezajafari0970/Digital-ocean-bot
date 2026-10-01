package app

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/resources"
)

func (c Container) syncVultrResourceRegistry(ctx context.Context, accountID string, servers []providers.Server) error {
	items := make([]resources.Resource, 0, len(servers))
	remote := make(map[string]bool, len(servers))
	for _, s := range servers {
		if s.ID == "" {
			continue
		}
		remote[s.ID] = true
		var managed bool
		_ = c.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM droplets WHERE account_id=$1 AND provider_resource_id=$2 AND state<>'DELETED')", accountID, s.ID).Scan(&managed)
		var expectedKey string
		_ = c.DB.QueryRowContext(ctx, "SELECT COALESCE(profile_snapshot->>'ssh_provider_key_id','') FROM deployments WHERE account_id=$1 AND provider_id=$2 ORDER BY created_at DESC LIMIT 1", accountID, s.ID).Scan(&expectedKey)
		actualKeys := stringSliceMetadata(s.Metadata["ssh_key_ids"])
		keyAttached, keyKnown := sshKeyAttachment(expectedKey, actualKeys)
		meta := map[string]any{"region": s.RegionID, "name": s.Name, "ready": s.Ready, "ipv4": s.PrimaryIPv4, "ssh_key_ids": actualKeys}
		if expectedKey != "" {
			meta["expected_ssh_key_id"] = expectedKey
		}
		if keyKnown {
			meta["ssh_key_attached"] = keyAttached
		}
		items = append(items, resources.Resource{AccountID: accountID, Provider: "vultr", ProviderResourceID: s.ID, Type: "server", State: string(s.State), Managed: managed, Metadata: meta})
	}
	if err := (resources.SQLRegistry{DB: c.DB}).Sync(ctx, accountID, items); err != nil {
		return err
	}
	rows, err := c.DB.QueryContext(ctx, "SELECT provider_resource_id FROM resources WHERE account_id=$1 AND provider='vultr' AND type='server' AND state<>'deleted'", accountID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var missing []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && !remote[id] {
			missing = append(missing, id)
		}
	}
	for _, id := range missing {
		_, _ = c.DB.ExecContext(ctx, "UPDATE resources SET state='deleted',managed=false,updated_at=now() WHERE account_id=$1 AND provider='vultr' AND type='server' AND provider_resource_id=$2", accountID, id)
	}
	return rows.Err()
}

func stringSliceMetadata(v any) []string {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...)
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func sshKeyAttachment(expected string, actual []string) (attached bool, known bool) {
	if expected == "" {
		return false, false
	}
	for _, id := range actual {
		if id == expected {
			return true, true
		}
	}
	return false, true
}
