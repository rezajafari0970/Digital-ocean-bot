package digitalocean

import "context"

type AccountInfo struct {
	Provider string
	Status   string
}

type Limits struct{}

func (c *Client) GetAccount(ctx context.Context) (*AccountInfo, error) {
	return &AccountInfo{Provider: "digitalocean", Status: "ready"}, nil
}

func (c *Client) GetLimits(ctx context.Context) (*Limits, error) {
	return &Limits{}, nil
}
