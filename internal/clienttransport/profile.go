// Package clienttransport applies bounded, reversible client export profiles.
// It must never be used to modify a Reality server inbound.
package clienttransport

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
)

type FragmentSettings struct {
	Packets  string `json:"packets"`
	Length   string `json:"length"`
	Delay    string `json:"delay"`
	MaxSplit string `json:"maxSplit"`
}
type Mask struct {
	Type     string           `json:"type"`
	Settings FragmentSettings `json:"settings"`
}
type FinalMask struct {
	TCP []Mask `json:"tcp"`
}

func Valid(preset string) bool {
	switch preset {
	case "off", "tls-record-v1", "tcp-balanced-v1", "layered-balanced-v1":
		return true
	}
	return false
}
func Build(preset string) (*FinalMask, error) {
	if !Valid(preset) {
		return nil, errors.New("unknown client transport preset")
	}
	if preset == "off" {
		return nil, nil
	}
	tls := Mask{"fragment", FragmentSettings{"tlshello", "100-200", "0", "3-6"}}
	tcp := Mask{"fragment", FragmentSettings{"1-1", "100-200", "0-1", "3-6"}}
	masks := []Mask{tls, tcp}
	if preset == "tls-record-v1" {
		masks = []Mask{tls}
	} else if preset == "tcp-balanced-v1" {
		masks = []Mask{tcp}
	}
	return &FinalMask{masks}, nil
}
func realityURI(raw string) (*url.URL, error) {
	u, e := url.Parse(raw)
	if e != nil {
		return nil, errors.New("invalid Reality URI")
	}
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil || u.Scheme != "vless" || u.User == nil || u.User.Username() == "" || u.Hostname() == "" || q.Get("security") != "reality" || (q.Get("type") != "tcp" && q.Get("type") != "raw") || q.Get("sni") == "" || q.Get("pbk") == "" {
		return nil, errors.New("unsupported Reality URI")
	}
	if _, hasPassword := u.User.Password(); hasPassword {
		return nil, errors.New("invalid VLESS identity")
	}
	port, e := strconv.Atoi(u.Port())
	if e != nil || port < 1 || port > 65535 {
		return nil, errors.New("invalid Reality port")
	}
	return u, nil
}

// Apply never rewrites an unconfigured URI. Stored snapshots remain untouched.
func Apply(raw, preset string) (string, error) {
	if preset == "" || preset == "off" {
		return raw, nil
	}
	mask, e := Build(preset)
	if e != nil {
		return "", e
	}
	u, e := realityURI(raw)
	if e != nil {
		return "", e
	}
	q := u.Query()
	if q.Has("fm") {
		return "", errors.New("existing client FinalMask must not be overwritten")
	}
	b, e := json.Marshal(mask)
	if e != nil {
		return "", e
	}
	q.Set("fm", string(b))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// XrayJSON exports one complete core configuration, with no direct fallback.
// Subscription clients must support an array of these custom configurations.
func XrayJSON(raw string) (map[string]any, error) {
	u, e := realityURI(raw)
	if e != nil {
		return nil, e
	}
	q := u.Query()
	port, _ := strconv.Atoi(u.Port())
	stream := map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{"serverName": q.Get("sni"), "fingerprint": q.Get("fp"), "publicKey": q.Get("pbk"), "shortId": q.Get("sid")}}
	if rawMask := q.Get("fm"); rawMask != "" {
		var m FinalMask
		if json.Unmarshal([]byte(rawMask), &m) != nil {
			return nil, errors.New("invalid FinalMask")
		}
		stream["finalmask"] = m
	}
	return map[string]any{
		"remarks": u.Fragment, "log": map[string]any{"loglevel": "warning"},
		"inbounds":  []any{map[string]any{"tag": "socks", "listen": "127.0.0.1", "port": 10808, "protocol": "socks", "settings": map[string]any{"auth": "noauth", "udp": true}}},
		"outbounds": []any{map[string]any{"tag": "proxy", "protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": u.Hostname(), "port": port, "users": []any{map[string]any{"id": u.User.Username(), "encryption": "none", "flow": q.Get("flow")}}}}}, "streamSettings": stream}},
	}, nil
}
