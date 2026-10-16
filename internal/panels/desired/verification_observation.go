package desired

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/postflight"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
)

// ErrVerificationObservation deliberately does not wrap ErrMismatch: a failed
// observation or invalid desired payload is not permission to change the panel.
var ErrVerificationObservation = errors.New("desired verification observation invalid")

func observationObject(v any) (map[string]any, error) {
	var b []byte
	var err error
	if text, ok := v.(string); ok {
		b = []byte(text)
	} else {
		b, err = json.Marshal(v)
	}
	var m map[string]any
	if err != nil || json.Unmarshal(b, &m) != nil || m == nil {
		return nil, ErrVerificationObservation
	}
	return m, nil
}

func observationStrings(m map[string]any, keys ...string) error {
	for _, key := range keys {
		if _, ok := m[key].(string); !ok {
			return fmt.Errorf("%w: %s", ErrVerificationObservation, key)
		}
	}
	return nil
}

func observationStringList(v any) error {
	b, err := json.Marshal(v)
	var list []string
	if err != nil || json.Unmarshal(b, &list) != nil || list == nil {
		return ErrVerificationObservation
	}
	return nil
}

func validateRepairObservation(in postflight.Inbound, id int64, p realityconfig.Payload) error {
	// Validate the expected side too; malformed builder data must not turn into
	// a mismatch that authorizes a native POST.
	b, err := json.Marshal(p)
	var expected postflight.Inbound
	if err != nil || json.Unmarshal(b, &expected) != nil {
		return ErrVerificationObservation
	}
	expected.ID = id
	if postflight.ValidatePayload(expected, id, p) != nil {
		return ErrVerificationObservation
	}
	if in.ID != id || in.ID <= 0 || in.Remark == "" || in.Port < 1 || in.Port > 65535 || in.Protocol != "vless" {
		return ErrVerificationObservation
	}
	s, err := observationObject(in.Settings)
	if err != nil {
		return err
	}
	if err = observationStrings(s, "decryption", "encryption"); err != nil {
		return err
	}
	clients, ok := s["clients"].([]any)
	if !ok || len(clients) == 0 {
		return ErrVerificationObservation
	}
	for _, value := range clients {
		c, err := observationObject(value)
		if err != nil {
			return err
		}
		if err = observationStrings(c, "id"); err != nil {
			return err
		}
		if flow, exists := c["flow"]; exists {
			if _, ok := flow.(string); !ok {
				return ErrVerificationObservation
			}
		}
	}
	st, err := observationObject(in.StreamSettings)
	if err != nil {
		return err
	}
	if err = observationStrings(st, "network", "security"); err != nil {
		return err
	}
	if st["security"] != "reality" {
		return ErrVerificationObservation
	}
	if st["network"] == "xhttp" {
		if _, err = transportprofile.FromStream(st); err != nil {
			return ErrVerificationObservation
		}
	}
	rs, err := observationObject(st["realitySettings"])
	if err != nil {
		return err
	}
	if err = observationStrings(rs, "dest", "privateKey"); err != nil {
		return err
	}
	for _, key := range []string{"serverNames", "shortIds"} {
		if err = observationStringList(rs[key]); err != nil {
			return err
		}
	}
	if p.Sniffing != nil {
		sniff, err := observationObject(in.Sniffing)
		if err != nil {
			return err
		}
		for _, key := range []string{"enabled", "metadataOnly", "routeOnly"} {
			if value, exists := sniff[key]; exists {
				if _, ok := value.(bool); !ok {
					return ErrVerificationObservation
				}
			}
		}
		if value, exists := sniff["destOverride"]; exists {
			if err = observationStringList(value); err != nil {
				return err
			}
		}
	}
	return nil
}
