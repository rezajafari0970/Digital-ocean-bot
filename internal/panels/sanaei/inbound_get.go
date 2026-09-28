package sanaei

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/postflight"
)

func GetInbound(ctx context.Context, exec SessionExecutor, remoteID int64) (postflight.Inbound, error) {
	if exec == nil || remoteID <= 0 {
		return postflight.Inbound{}, ErrMutationRequest
	}
	r, e := exec.Do(ctx, SessionRequest{Method: http.MethodGet, Path: fmt.Sprintf("panel/api/inbounds/get/%d", remoteID)})
	if e != nil {
		return postflight.Inbound{}, e
	}
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return postflight.Inbound{}, ErrMutationRequest
	}
	var env struct {
		Success bool               `json:"success"`
		Obj     postflight.Inbound `json:"obj"`
	}
	if json.Unmarshal(r.Body, &env) != nil || !env.Success {
		return postflight.Inbound{}, ErrMutationRejected
	}
	return env.Obj, nil
}
