package sanaei

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type ClientTraffic struct {
	ID         int64  `json:"id"`
	InboundID  int    `json:"inboundId"`
	Enable     bool   `json:"enable"`
	Email      string `json:"email"`
	Up         int64  `json:"up"`
	Down       int64  `json:"down"`
	Total      int64  `json:"total"`
	ExpiryTime int64  `json:"expiryTime"`
}
type trafficResponse struct {
	Success bool          `json:"success"`
	Obj     ClientTraffic `json:"obj"`
}

func (c *APIClient) GetClientTraffic(ctx context.Context, email string) (ClientTraffic, error) {
	var r trafficResponse
	if err := c.json(ctx, http.MethodGet, "/panel/api/inbounds/getClientTraffics/"+url.PathEscape(email), nil, &r); err != nil {
		return ClientTraffic{}, err
	}
	if !r.Success {
		return ClientTraffic{}, fmt.Errorf("%w: traffic lookup rejected", ErrAPIRequest)
	}
	return r.Obj, nil
}
