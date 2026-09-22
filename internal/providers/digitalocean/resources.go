package digitalocean

import (
	"context"
	"strconv"
)

type Resource struct {
	ID    string
	Type  string
	State string
}

func (c *Client) ListResources(ctx context.Context) ([]Resource, error) {
	d, err := c.ListDropletModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Resource, 0, len(d))
	for _, x := range d {
		out = append(out, Resource{ID: strconv.Itoa(x.ID), Type: "droplet", State: x.Status})
	}
	return out, nil
}

func (c *Client) ListDroplets(ctx context.Context) ([]Resource, error) { return c.ListResources(ctx) }
func (c *Client) ListDropletModels(ctx context.Context) ([]Droplet, error) {
	var out []Droplet
	err := c.listAll(ctx, "/droplets", "droplets", &out)
	for i := range out {
		for _, n := range out[i].Networks.V4 {
			if n.Type == "public" {
				out[i].PublicIPv4 = n.IPAddress
				break
			}
		}
	}
	return out, err
}
