package digitalocean

import "time"

func NewSnapshot(accountID string, result DiscoveryResult) Snapshot {
	return Snapshot{
		AccountID: accountID,
		Provider:  "digitalocean",
		Version:   1,
		Data: map[string]any{
			"account":   result.Account,
			"limits":    result.Limits,
			"resources": result.Resources,
		},
		CreatedAt: time.Now().UTC(),
	}
}
