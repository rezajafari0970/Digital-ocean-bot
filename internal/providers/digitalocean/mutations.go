package digitalocean

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type CreateDropletRequest struct {
	Name       string   `json:"name"`
	Region     string   `json:"region"`
	Size       string   `json:"size"`
	Image      any      `json:"image"`
	SSHKeys    []any    `json:"ssh_keys,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Monitoring bool     `json:"monitoring,omitempty"`
	IPv6       bool     `json:"ipv6,omitempty"`
}

type SSHKeyCreateRequest struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
}
type SSHKeyCreated struct {
	ID                           int `json:"id"`
	Name, Fingerprint, PublicKey string
}

func (c *Client) CreateSSHKey(ctx context.Context, name, publicKey string) (SSHKeyCreated, error) {
	var e struct {
		SSHKey SSHKeyCreated `json:"ssh_key"`
	}
	err := c.request(ctx, http.MethodPost, "/account/keys", SSHKeyCreateRequest{Name: name, PublicKey: publicKey}, &e)
	return e.SSHKey, err
}
func (c *Client) DeleteSSHKey(ctx context.Context, id int) error {
	return c.request(ctx, http.MethodDelete, "/account/keys/"+strconv.Itoa(id), nil, nil)
}

type Action struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
	Type   string `json:"type"`
}
type createDropletEnvelope struct {
	Droplet Droplet        `json:"droplet"`
	Links   map[string]any `json:"links"`
}
type actionEnvelope struct {
	Action Action `json:"action"`
}

func (c *Client) request(ctx context.Context, method, path string, body any, out any) error {
	if err := c.Validate(c.Account.AccountID); err != nil {
		return err
	}
	token, err := c.Secrets.Get(ctx, c.Account.AccountID, c.TokenRef)
	if err != nil {
		return err
	}
	defer zero(token)
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			return ErrProviderRequest
		}
		reader = bytes.NewReader(raw)
	}
	u, err := url.JoinPath(strings.TrimRight(c.BaseURL, "/"), path)
	if err != nil {
		return ErrProviderRequest
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return ErrProviderRequest
	}
	req.Header.Set("Authorization", "Bearer "+string(token))
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: transport", ErrProviderRequest)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var body struct {
			ID      string `json:"id"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return HTTPError{Status: resp.StatusCode, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")), Path: req.URL.String(), Code: body.ID, Message: body.Message}
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("%w: decode", ErrProviderRequest)
		}
	}
	return nil
}

func (c *Client) CreateDroplet(ctx context.Context, req CreateDropletRequest) (Droplet, error) {
	var e createDropletEnvelope
	err := c.request(ctx, http.MethodPost, "/droplets", req, &e)
	return e.Droplet, err
}
func (c *Client) DeleteDroplet(ctx context.Context, id int) error {
	return c.request(ctx, http.MethodDelete, "/droplets/"+strconv.Itoa(id), nil, nil)
}
func (c *Client) GetAction(ctx context.Context, id int) (Action, error) {
	var e actionEnvelope
	err := c.get(ctx, "/actions/"+strconv.Itoa(id), &e)
	return e.Action, err
}
