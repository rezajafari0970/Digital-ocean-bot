package digitalocean

import (
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/accounts"
)

var ErrMissingContext = errors.New("provider account context required")

type Client struct {
	Account     accounts.Context
	TokenSecret string
}

func NewClient(ctx accounts.Context, secretRef string) (*Client, error) {
	if ctx.AccountID == "" || secretRef == "" {
		return nil, ErrMissingContext
	}
	return &Client{Account: ctx, TokenSecret: secretRef}, nil
}

func (c *Client) Validate(accountID string) error {
	return c.Account.Authorize(accountID)
}
