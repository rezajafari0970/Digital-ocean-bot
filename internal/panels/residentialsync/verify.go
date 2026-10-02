package residentialsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func readXraySetting(ctx context.Context, exec sanaei.SSHSessionExecutorV2) (map[string]any, error) {
	resp, err := exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/xray/", TimeoutSeconds: 10})
	if err != nil {
		return nil, err
	}
	var top struct {
		Success bool   `json:"success"`
		Obj     string `json:"obj"`
	}
	if json.Unmarshal(resp.Body, &top) != nil || !top.Success {
		return nil, errors.New("xray read")
	}
	var inner struct {
		XraySetting map[string]any `json:"xraySetting"`
	}
	if json.Unmarshal([]byte(top.Obj), &inner) != nil || inner.XraySetting == nil {
		return nil, errors.New("xray parse")
	}
	return inner.XraySetting, nil
}

func desiredResidentialApplied(setting map[string]any, ps []rp) bool {
	if setting == nil {
		return false
	}
	outs, _ := setting["outbounds"].([]any)
	for _, want := range ps {
		found := false
		wantProto := want.Type
		if wantProto == "socks5" {
			wantProto = "socks"
		}
		for _, raw := range outs {
			m, ok := raw.(map[string]any)
			if !ok || fmt.Sprint(m["tag"]) != want.Tag || fmt.Sprint(m["protocol"]) != wantProto {
				continue
			}
			if wantProto == "socks" {
				settings, _ := m["settings"].(map[string]any)
				if udp, _ := settings["udp"].(bool); !udp {
					continue
				}
			}
			found = true
			break
		}
		if !found {
			return false
		}
	}
	routing, _ := setting["routing"].(map[string]any)
	rules, _ := routing["rules"].([]any)
	if len(ps) == 0 {
		for _, raw := range rules {
			m, ok := raw.(map[string]any)
			if ok && fmt.Sprint(m["ruleTag"]) == "dob-residential-ads" {
				return false
			}
		}
		return true
	}
	wantNetwork := "tcp"
	if ps[0].Type == "socks5" {
		wantNetwork = "tcp,udp"
	}
	for _, raw := range rules {
		m, ok := raw.(map[string]any)
		if ok && fmt.Sprint(m["ruleTag"]) == "dob-residential-ads" &&
			fmt.Sprint(m["outboundTag"]) == ps[0].Tag &&
			fmt.Sprint(m["network"]) == wantNetwork {
			return true
		}
	}
	return false
}
