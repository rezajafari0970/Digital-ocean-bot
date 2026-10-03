package residentialsync

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func readXraySetting(ctx context.Context, exec sanaei.SessionExecutor) (map[string]any, string, error) {
	resp, err := exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/xray/", TimeoutSeconds: 10})
	if err != nil {
		return nil, "", err
	}
	var top struct {
		Success bool
		Obj     json.RawMessage
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || json.Unmarshal(resp.Body, &top) != nil || !top.Success {
		return nil, "", errors.New("xray read failed")
	}
	obj, err := decodeObject(top.Obj)
	if err != nil {
		return nil, "", err
	}
	raw, err := json.Marshal(obj["xraySetting"])
	if err != nil {
		return nil, "", err
	}
	setting, err := decodeObject(raw)
	testURL, _ := obj["outboundTestUrl"].(string)
	return setting, testURL, err
}
