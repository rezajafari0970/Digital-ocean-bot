package transportprofile

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ClientTCPMask preserves the modern per-segment schedule and adds the scalar
// fields required by pre-array Xray cores (verified with 26.5.9). Those cores
// repeat the first range; they cannot execute the full per-segment schedule.
// Server configuration and persisted profiles continue to use TCPMask.
func (c Config) ClientTCPMask() (json.RawMessage, error) {
	raw, err := c.TCPMask()
	if err != nil {
		return nil, err
	}
	var wire struct {
		TCP []struct {
			Type     string                     `json:"type"`
			Settings map[string]json.RawMessage `json:"settings"`
		} `json:"tcp"`
	}
	if err = json.Unmarshal(raw, &wire); err != nil {
		return nil, err
	}
	for _, mask := range wire.TCP {
		for _, pair := range [][2]string{{"lengths", "length"}, {"delays", "delay"}} {
			if _, supplied := mask.Settings[pair[1]]; supplied {
				continue
			}
			var values []json.RawMessage
			if err = json.Unmarshal(mask.Settings[pair[0]], &values); err != nil || len(values) == 0 {
				return nil, fmt.Errorf("%w: missing fragment %s", ErrInvalid, pair[0])
			}
			// Never publish an ambiguous null scalar or schedule entry.
			for _, value := range values {
				if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					return nil, fmt.Errorf("%w: null fragment %s", ErrInvalid, pair[0])
				}
			}
			mask.Settings[pair[1]] = values[0]
		}
	}
	return json.Marshal(wire)
}
