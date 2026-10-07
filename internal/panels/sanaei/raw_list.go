package sanaei

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

func ReadRawInboundList(ctx context.Context, exec SessionExecutor) ([]json.RawMessage, error) {
	resp, err := exec.Do(ctx, SessionRequest{Method: http.MethodGet, Path: "panel/api/inbounds/list", TimeoutSeconds: 12})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, ErrSessionRequest
	}
	var env inventoryEnvelope
	if err = json.Unmarshal(resp.Body, &env); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInventoryRejected, err)
	}
	if !env.Success || env.Obj == nil {
		return nil, ErrInventoryRejected
	}
	return env.Obj, nil
}
