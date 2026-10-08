package digitalocean

import (
	"context"
	"strings"
	"unicode"
)

type AccountInfo struct {
	UUID            string `json:"uuid"`
	Email           string `json:"email"`
	Status          string `json:"status"`
	StatusMessage   string `json:"status_message"`
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
	e.Account.StatusMessage = boundedStatusMessage(e.Account.StatusMessage)
	return &e.Account, nil
}

// Provider text is display data, never authorization or HTML.
func boundedStatusMessage(s string) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s))
	r := []rune(s)
	if len(r) > 512 {
		s = string(r[:512]) + "…"
	}
	return s
}
func (c *Client) GetLimits(ctx context.Context) (*Limits, error) {
	a, err := c.GetAccount(ctx)
	if err != nil {
		return nil, err
	}
	return &Limits{DropletLimit: a.DropletLimit, VolumeLimit: a.VolumeLimit, ReservedIPLimit: a.ReservedIPLimit}, nil
}
