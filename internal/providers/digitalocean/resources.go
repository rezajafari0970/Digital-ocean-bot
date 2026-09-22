package digitalocean

import "context"

type Resource struct{}

func (c *Client) ListResources(ctx context.Context) ([]Resource, error) {
	return []Resource{}, nil
}

func (c *Client) ListDroplets(ctx context.Context) ([]Resource, error) {
	return c.ListResources(ctx)
}
