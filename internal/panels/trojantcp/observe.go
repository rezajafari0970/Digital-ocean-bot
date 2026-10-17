package trojantcp

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

func Observe(raws []json.RawMessage, p Policy, host string, clients []Client, remote sql.NullInt64) (int64, error) {
	desired, e := Build(p, host, clients)
	if e != nil {
		return 0, e
	}
	remark := desired["remark"].(string)
	var result int64
	for _, raw := range raws {
		var in struct {
			ID                                 int64
			Remark, Protocol, Listen           string
			Port                               int
			Enable                             bool
			Settings, StreamSettings, Sniffing json.RawMessage
		}
		if json.Unmarshal(raw, &in) != nil || in.ID <= 0 {
			return 0, errors.New("trojan_inventory_invalid")
		}
		if in.Remark != remark {
			if in.Port == p.Port {
				return 0, errors.New("trojan_port_occupied")
			}
			if remote.Valid && in.ID == remote.Int64 {
				return 0, errors.New("trojan_listener_identity_changed")
			}
			continue
		}
		if result != 0 || in.Port != p.Port || in.Protocol != "trojan" || !in.Enable || in.Listen != "" && in.Listen != "0.0.0.0" {
			return 0, errors.New("trojan_listener_conflict")
		}
		if remote.Valid && remote.Int64 != in.ID {
			return 0, errors.New("trojan_listener_identity_changed")
		}
		settings, e := decode(in.Settings)
		if e != nil {
			return 0, errors.New("trojan_settings_invalid")
		}
		stream, e := decode(in.StreamSettings)
		if e != nil {
			return 0, errors.New("trojan_stream_invalid")
		}
		if stream["network"] != "tcp" || stream["security"] != "tls" {
			return 0, errors.New("trojan_transport_mismatch")
		}
		tls, ok := stream["tlsSettings"].(map[string]any)
		if !ok {
			return 0, errors.New("trojan_tls_settings_missing")
		}
		certs, ok := tls["certificates"].([]any)
		if !ok || len(certs) != 1 {
			return 0, errors.New("trojan_certificate_config_mismatch")
		}
		cert, ok := certs[0].(map[string]any)
		if !ok || cert["certificateFile"] != CertificateFile || cert["keyFile"] != KeyFile || cert["oneTimeLoading"] == true {
			return 0, errors.New("trojan_certificate_config_mismatch")
		}
		got, ok := settings["clients"].([]any)
		if !ok || len(got) != len(clients) {
			return 0, errors.New("trojan_client_membership_mismatch")
		}
		expected := map[string]Client{}
		for _, c := range clients {
			expected[c.ID] = c
		}
		seen := map[string]bool{}
		for _, v := range got {
			c, ok := v.(map[string]any)
			if !ok {
				return 0, errors.New("trojan_client_invalid")
			}
			id, _ := c["id"].(string)
			want, ok := expected[id]
			if !ok || seen[id] || c["email"] != want.Email || c["password"] != want.Password || c["enable"] != true {
				return 0, errors.New("trojan_client_identity_mismatch")
			}
			if flow, _ := c["flow"].(string); flow != "" {
				return 0, errors.New("trojan_unexpected_flow")
			}
			seen[id] = true
		}
		sniff, e := decode(in.Sniffing)
		if e != nil || sniff["enabled"] != true || sniff["metadataOnly"] == true {
			return 0, errors.New("trojan_sniffing_mismatch")
		}
		overrides, _ := sniff["destOverride"].([]any)
		for _, want := range []string{"http", "tls", "quic"} {
			found := false
			for _, x := range overrides {
				found = found || reflect.DeepEqual(x, want)
			}
			if !found {
				return 0, fmt.Errorf("trojan_sniffing_mismatch")
			}
		}
		result = in.ID
	}
	return result, nil
}
