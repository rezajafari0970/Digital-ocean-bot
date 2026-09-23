package digitalocean

import "context"

func (c *Client) Catalog(ctx context.Context) (DiscoveryResult, error) {
	a, err := c.GetAccount(ctx)
	if err != nil {
		return DiscoveryResult{}, err
	}
	r := DiscoveryResult{Account: *a, Limits: Limits{DropletLimit: a.DropletLimit, VolumeLimit: a.VolumeLimit, ReservedIPLimit: a.ReservedIPLimit}}
	if err = c.listAll(ctx, "/regions", "regions", &r.Regions); err != nil {
		return DiscoveryResult{}, err
	}
	if err = c.listAll(ctx, "/sizes", "sizes", &r.Sizes); err != nil {
		return DiscoveryResult{}, err
	}
	if err = c.listAll(ctx, "/images?type=distribution", "images", &r.Images); err != nil {
		return DiscoveryResult{}, err
	}
	return r, nil
}
