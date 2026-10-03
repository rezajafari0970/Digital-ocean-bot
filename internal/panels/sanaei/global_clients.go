package sanaei

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// GlobalClient retains only the identity/policy fields needed for verification.
// The installed v3 /clients/list response is an array of these global records.
type GlobalClient struct {
	Traffic    *ClientTraffic `json:"traffic"`
	UUID       string         `json:"uuid"`
	Email      string         `json:"email"`
	Enable     bool           `json:"enable"`
	TotalGB    int64          `json:"totalGB"`
	ExpiryTime int64          `json:"expiryTime"`
	LimitHWID  int            `json:"limitHwid"`
	InboundIDs []int64        `json:"inboundIds"`
}

func ReadGlobalClientsSession(ctx context.Context, exec SessionExecutor) ([]GlobalClient, error) {
	if exec == nil {
		return nil, ErrMutationRequest
	}
	resp, err := exec.Do(ctx, SessionRequest{Method: http.MethodGet, Path: "panel/api/clients/list", TimeoutSeconds: 30})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: global list http=%d", ErrInventoryRejected, resp.StatusCode)
	}
	var v struct {
		Success bool            `json:"success"`
		Obj     *[]GlobalClient `json:"obj"`
	}
	if json.Unmarshal(resp.Body, &v) != nil || !v.Success || v.Obj == nil {
		return nil, ErrInventoryRejected
	}
	return *v.Obj, nil
}
