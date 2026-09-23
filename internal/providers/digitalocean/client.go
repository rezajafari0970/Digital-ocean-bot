package digitalocean

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/accounts"
)

var ErrMissingContext = errors.New("provider account context required")
var ErrProviderRequest = errors.New("digitalocean provider request failed")

type SecretReader interface {
	Get(context.Context, string, string) ([]byte, error)
}

type Client struct {
	Account  accounts.Context
	TokenRef string
	Secrets  SecretReader
	HTTP     *http.Client
	BaseURL  string
}

func NewClient(ctx accounts.Context, tokenRef string, secrets SecretReader, httpClient *http.Client) (*Client, error) {
	if ctx.AccountID == "" || tokenRef == "" || secrets == nil || httpClient == nil {
		return nil, ErrMissingContext
	}
	return &Client{Account: ctx, TokenRef: tokenRef, Secrets: secrets, HTTP: httpClient, BaseURL: "https://api.digitalocean.com/v2"}, nil
}

func (c *Client) Validate(accountID string) error { return c.Account.Authorize(accountID) }

func (c *Client) get(ctx context.Context, path string, out any) error {
	if err := c.Validate(c.Account.AccountID); err != nil {
		return err
	}
	token, err := c.Secrets.Get(ctx, c.Account.AccountID, c.TokenRef)
	if err != nil {
		return err
	}
	defer zero(token)
	base, err := url.Parse(strings.TrimRight(c.BaseURL, "/") + "/")
	if err != nil {
		return ErrProviderRequest
	}
	rel, err := url.Parse(strings.TrimLeft(path, "/"))
	if err != nil {
		return ErrProviderRequest
	}
	u := base.String() + rel.String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ErrProviderRequest
	}
	req.Header.Set("Authorization", "Bearer "+string(token))
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: transport: %v", ErrProviderRequest, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return HTTPError{Status: resp.StatusCode, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")), Path: u}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: decode", ErrProviderRequest)
	}
	return nil
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
