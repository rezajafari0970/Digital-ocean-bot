package configdriver

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
)

// Outbound is used only for an authenticated, read-only live connection probe.
// Credentials travel over encrypted SSH stdin and never enter shell arguments.
func (r *Registry) Outbound(s Spec, host string, c Credentials, cl Client) (map[string]any, error) {
	if _, e := r.Build(s, host, "probe", c); e != nil {
		return nil, e
	}
	stream := map[string]any{"network": s.Network, "security": s.Security}
	for k, v := range r.transports[s.Network].Server(s) {
		stream[k] = v
	}
	if s.Network == "grpc" && s.Host != "" {
		stream["grpcSettings"].(map[string]any)["authority"] = s.Host
	}
	if s.Security == "tls" {
		stream["tlsSettings"] = map[string]any{"serverName": host}
	}
	if s.Security == "reality" {
		stream["realitySettings"] = map[string]any{"serverName": s.SNI, "publicKey": c.PublicKey, "shortId": c.ShortID, "fingerprint": "chrome"}
	}
	if cl.FinalMask != "" {
		mask, e := (transportprofile.Config{FinalMask: json.RawMessage(cl.FinalMask)}).ClientTCPMask()
		if e != nil {
			return nil, e
		}
		var v any
		if e = json.Unmarshal(mask, &v); e != nil {
			return nil, e
		}
		stream["finalmask"] = v
	}
	settings := map[string]any{}
	switch s.Protocol {
	case "vless":
		settings["vnext"] = []any{map[string]any{"address": "127.0.0.1", "port": s.Port, "users": []any{map[string]any{"id": cl.ID, "encryption": "none", "flow": ClientFlow(s, cl)}}}}
	case "vmess":
		settings["vnext"] = []any{map[string]any{"address": "127.0.0.1", "port": s.Port, "users": []any{map[string]any{"id": cl.ID, "security": "auto"}}}}
	case "trojan":
		settings["servers"] = []any{map[string]any{"address": "127.0.0.1", "port": s.Port, "password": cl.Password}}
	case "shadowsocks":
		settings["servers"] = []any{map[string]any{"address": "127.0.0.1", "port": s.Port, "password": cl.Password, "method": "aes-128-gcm"}}
	}
	return map[string]any{"protocol": s.Protocol, "settings": settings, "streamSettings": stream}, nil
}

// PanelBody adapts canonical protocol fields to Sanaei's persisted Client model.
func PanelBody(payload map[string]any) map[string]any {
	body := normalized(payload).(map[string]any)
	if body["protocol"] == "shadowsocks" {
		settings := body["settings"].(map[string]any)
		for _, v := range settings["clients"].([]any) {
			c := v.(map[string]any)
			c["security"] = c["method"]
			delete(c, "method")
		}
	}
	return body
}
