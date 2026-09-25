package digitalocean

import (
	"context"
	"errors"
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
func (c *Client) DropletExists(ctx context.Context, id int) (bool, error) {
	_, err := c.GetDroplet(ctx, id)
	if err == nil {
		return true, nil
	}
	var h HTTPError
	if errors.As(err, &h) && h.Status == 404 {
		return false, nil
	}
	return false, err
}
func (c *Client) GetDroplet(ctx context.Context, id int) (Droplet, error) {
	var out struct {
		Droplet Droplet `json:"droplet"`
	}
	if err := c.get(ctx, "/droplets/"+strconv.Itoa(id), &out); err != nil {
		return Droplet{}, err
	}
	for _, n := range out.Droplet.Networks.V4 {
		if n.Type == "public" {
			out.Droplet.PublicIPv4 = n.IPAddress
			break
		}
	}
	return out.Droplet, nil
}
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
