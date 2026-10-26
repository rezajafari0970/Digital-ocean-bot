package transportprofile

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// ProfileMask is the complete client contract. TCPMask intentionally selects
// only the active TCP layer for the legacy server profile; independent profiles
// must retain the operator's UDP header as well, even on a TCP transport.
func (c Config) ProfileMask() (json.RawMessage, error) {
	m, err := jsonObject(c.FinalMask)
	if err != nil {
		return nil, fmt.Errorf("FinalMask must be a JSON object of at most 32768 bytes")
	}
	tcp, err := c.ClientTCPMask()
	if err != nil {
		return nil, err
	}
	if raw, present := m["udp"]; present {
		var masks []struct {
			Type     string          `json:"type"`
			Settings json.RawMessage `json:"settings"`
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if dec.Decode(&masks) != nil || masks == nil || len(masks) > 8 {
			return nil, fmt.Errorf("FinalMask udp must be an array of at most 8 masks")
		}
		for i, mask := range masks {
			if mask.Type != "header-custom" {
				return nil, fmt.Errorf("FinalMask udp[%d]: supported type is header-custom", i)
			}
			settings, e := jsonObject(mask.Settings)
			if e != nil || len(settings) == 0 {
				return nil, fmt.Errorf("FinalMask udp[%d].settings must contain client or server headers", i)
			}
			for direction, raw := range settings {
				if direction != "client" && direction != "server" {
					return nil, fmt.Errorf("FinalMask udp[%d]: unknown header direction %q", i, direction)
				}
				var headers []json.RawMessage
				if json.Unmarshal(raw, &headers) != nil || headers == nil || len(headers) > 64 {
					return nil, fmt.Errorf("FinalMask udp[%d].%s must contain at most 64 headers", i, direction)
				}
				for j, header := range headers {
					if e := validateUDPHeader(header); e != nil {
						return nil, fmt.Errorf("FinalMask udp[%d].%s[%d]: %w", i, direction, j, e)
					}
				}
			}
		}
	}
	// QUIC is not a registered profile transport. Empty legacy placeholders are
	// retained; nonempty settings must not be silently ignored or claim support.
	if raw, present := m["quicParams"]; present {
		q, e := jsonObject(raw)
		if e != nil || len(q) != 0 {
			return nil, fmt.Errorf("FinalMask quicParams requires a QUIC transport; leave it empty for this profile")
		}
	}
	var effective map[string]json.RawMessage
	if err = json.Unmarshal(tcp, &effective); err != nil {
		return nil, err
	}
	m["tcp"] = effective["tcp"]
	return json.Marshal(m)
}

func validateUDPHeader(raw json.RawMessage) error {
	m, err := jsonObject(raw)
	if err != nil || len(m) == 0 {
		return fmt.Errorf("header must be a nonempty object")
	}
	for k, v := range m {
		switch k {
		case "rand":
			if !integer(v, 0, 8192) {
				return fmt.Errorf("rand must be an integer from 0 to 8192")
			}
		case "randRange":
			if !rangeValue(v, 0, 255) {
				return fmt.Errorf("randRange must be within 0-255")
			}
		case "type", "packet":
		default:
			return fmt.Errorf("unknown header field %q", k)
		}
	}
	packet, hasPacket := m["packet"]
	_, hasRand := m["rand"]
	if hasPacket && hasRand {
		return fmt.Errorf("packet and rand cannot be combined")
	}
	if !hasPacket && !hasRand {
		return fmt.Errorf("specify rand or packet")
	}
	typ := "array"
	if v, ok := m["type"]; ok {
		if json.Unmarshal(v, &typ) != nil || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return fmt.Errorf("type must be a string")
		}
		if typ == "" {
			typ = "array"
		}
	}
	if typ != "array" && typ != "str" && typ != "hex" && typ != "base64" {
		return fmt.Errorf("unsupported packet type")
	}
	if !hasPacket {
		return nil
	}
	if typ == "array" {
		var items []json.RawMessage
		if json.Unmarshal(packet, &items) != nil || items == nil || len(items) > 8192 {
			return fmt.Errorf("packet must be a byte array")
		}
		for _, v := range items {
			if !integer(v, 0, 255) {
				return fmt.Errorf("packet bytes must be within 0-255")
			}
		}
		return nil
	}
	var value string
	if json.Unmarshal(packet, &value) != nil || bytes.Equal(bytes.TrimSpace(packet), []byte("null")) || len(value) > 16384 {
		return fmt.Errorf("packet must be a bounded string")
	}
	if typ == "hex" {
		if _, e := hex.DecodeString(value); e != nil {
			return fmt.Errorf("invalid hex packet")
		}
	}
	if typ == "base64" {
		if _, e := base64.StdEncoding.DecodeString(value); e != nil {
			return fmt.Errorf("invalid base64 packet")
		}
	}
	return nil
}
