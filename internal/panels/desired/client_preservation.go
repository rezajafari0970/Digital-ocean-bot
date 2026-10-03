package desired

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
)

// Raw messages retain unknown client fields and integer precision.
func preserveInboundClients(payload realityconfig.Payload, current any) (realityconfig.Payload, error) {
	var data []byte
	var err error
	if str, ok := current.(string); ok {
		data = []byte(str)
	} else {
		data, err = json.Marshal(current)
	}
	if err != nil {
		return payload, err
	}
	var settings map[string]json.RawMessage
	if json.Unmarshal(data, &settings) != nil {
		return payload, errors.New("structural update: invalid live settings")
	}
	raw, ok := settings["clients"]
	var clients []json.RawMessage
	if !ok || string(raw) == "null" || json.Unmarshal(raw, &clients) != nil {
		return payload, errors.New("structural update: missing live clients")
	}
	preserved := make(map[string]any, len(settings)+len(payload.Settings))
	for k, v := range settings {
		preserved[k] = v
	}
	for k, v := range payload.Settings {
		if k != "clients" {
			preserved[k] = v
		}
	}
	preserved["clients"] = raw
	payload.Settings = preserved
	return payload, nil
}

func liveInboundSettings(ctx context.Context, exec sanaei.SessionExecutor, id int64) (any, error) {
	r, err := exec.Do(ctx, sanaei.SessionRequest{Method: "GET", Path: fmt.Sprintf("panel/api/inbounds/get/%d", id), TimeoutSeconds: 30})
	if err != nil {
		return nil, err
	}
	var env struct {
		Success bool
		Obj     struct {
			ID       int64
			Settings json.RawMessage
		}
	}
	if r.StatusCode < 200 || r.StatusCode >= 300 || json.Unmarshal(r.Body, &env) != nil || !env.Success || env.Obj.ID != id {
		return nil, errors.New("structural update: live inbound unavailable")
	}
	var text string
	if json.Unmarshal(env.Obj.Settings, &text) == nil {
		return text, nil
	}
	return env.Obj.Settings, nil
}
