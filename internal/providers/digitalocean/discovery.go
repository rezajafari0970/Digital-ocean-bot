package digitalocean

import "context"

func (c *Client) Discover(ctx context.Context) (DiscoveryResult, error) {
	if err := c.Validate(c.Account.AccountID); err != nil {
		return DiscoveryResult{}, err
	}
	return DiscoveryResult{
		Account:   map[string]any{},
		Limits:    map[string]any{},
		Regions:   []map[string]any{},
		Sizes:     []map[string]any{},
		Images:    []map[string]any{},
		Resources: []map[string]any{},
	}, nil
}
