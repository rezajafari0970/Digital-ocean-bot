package digitalocean

import "context"

type AccountInfo struct {
	UUID            string `json:"uuid"`
	Email           string `json:"email"`
	Status          string `json:"status"`
	EmailVerified   bool   `json:"email_verified"`
	DropletLimit    int    `json:"droplet_limit"`
	VolumeLimit     int    `json:"volume_limit"`
	ReservedIPLimit int    `json:"reserved_ip_limit"`
}

type Limits struct {
	DropletLimit    int
	VolumeLimit     int
	ReservedIPLimit int
}
type accountEnvelope struct {
	Account AccountInfo `json:"account"`
}

func (c *Client) GetAccount(ctx context.Context) (*AccountInfo, error) {
	var e accountEnvelope
	if err := c.get(ctx, "/account", &e); err != nil {
		return nil, err
	}
	return &e.Account, nil
}
func (c *Client) GetLimits(ctx context.Context) (*Limits, error) {
	a, err := c.GetAccount(ctx)
	if err != nil {
		return nil, err
	}
	return &Limits{DropletLimit: a.DropletLimit, VolumeLimit: a.VolumeLimit, ReservedIPLimit: a.ReservedIPLimit}, nil
}
