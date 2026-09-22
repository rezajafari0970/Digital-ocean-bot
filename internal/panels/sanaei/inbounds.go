package sanaei

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Inbound struct {
	ID       int    `json:"id"`
	Remark   string `json:"remark"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Settings string `json:"settings"`
}
type inboundListResponse struct {
	Success bool      `json:"success"`
	Msg     string    `json:"msg"`
	Obj     []Inbound `json:"obj"`
}
type genericResponse struct {
	Success bool   `json:"success"`
	Msg     string `json:"msg"`
}

func (c *APIClient) ListInbounds(ctx context.Context) ([]Inbound, error) {
	var r inboundListResponse
	if err := c.json(ctx, http.MethodGet, "/panel/api/inbounds/list", nil, &r); err != nil {
		return nil, err
	}
	if !r.Success {
		return nil, fmt.Errorf("%w: api rejected", ErrAPIRequest)
	}
	return r.Obj, nil
}

func (c *APIClient) AddClient(ctx context.Context, inboundID int, client Client) error {
	payload, err := json.Marshal(map[string]any{"clients": []Client{client}})
	if err != nil {
		return err
	}
	form := url.Values{"id": {fmt.Sprintf("%d", inboundID)}, "settings": {string(payload)}}
	var r genericResponse
	if err := c.json(ctx, http.MethodPost, "/panel/api/inbounds/addClient", form, &r); err != nil {
		return err
	}
	if !r.Success {
		return fmt.Errorf("%w: api rejected", ErrAPIRequest)
	}
	return nil
}

func (c *APIClient) DeleteClient(ctx context.Context, inboundID int, clientID string) error {
	var r genericResponse
	if err := c.json(ctx, http.MethodPost, fmt.Sprintf("/panel/api/inbounds/%d/delClient/%s", inboundID, url.PathEscape(clientID)), nil, &r); err != nil {
		return err
	}
	if !r.Success {
		return fmt.Errorf("%w: api rejected", ErrAPIRequest)
	}
	return nil
}
