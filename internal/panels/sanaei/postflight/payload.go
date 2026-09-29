package postflight

import (
	"encoding/json"
	"fmt"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
)

func ValidatePayload(in Inbound, remoteID int64, p realityconfig.Payload) error {
	b, _ := json.Marshal(p.Settings)
	var s map[string]any
	if json.Unmarshal(b, &s) != nil {
		return ErrMismatch
	}
	cs, ok := s["clients"].([]any)
	if !ok || len(cs) != 1 {
		return ErrMismatch
	}
	c, e := object(cs[0])
	if e != nil {
		return e
	}
	b, _ = json.Marshal(p.StreamSettings)
	var st map[string]any
	if json.Unmarshal(b, &st) != nil {
		return ErrMismatch
	}
	rs, e := object(st["realitySettings"])
	if e != nil {
		return e
	}
	var names []string
	b, _ = json.Marshal(rs["serverNames"])
	if json.Unmarshal(b, &names) != nil {
		return ErrMismatch
	}
	var ids []string
	b, _ = json.Marshal(rs["shortIds"])
	if json.Unmarshal(b, &ids) != nil || len(ids) != 1 {
		return ErrMismatch
	}
	return Validate(in, Expected{RemoteID: remoteID, Remark: p.Remark, Port: p.Port, UUID: fmt.Sprint(c["id"]), Flow: fmt.Sprint(c["flow"]), Target: fmt.Sprint(rs["dest"]), ServerNames: names, PrivateKey: fmt.Sprint(rs["privateKey"]), ShortID: ids[0], Fingerprint: fmt.Sprint(rs["fingerprint"]), Sniffing: p.Sniffing})
}
