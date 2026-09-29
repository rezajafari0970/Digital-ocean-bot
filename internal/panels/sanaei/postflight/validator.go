package postflight

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

var ErrMismatch = errors.New("sanaei inbound semantic mismatch")

type Expected struct {
	RemoteID    int64
	Remark      string
	Port        int
	UUID        string
	Flow        string
	Target      string
	ServerNames []string
	PrivateKey  string
	ShortID     string
	Fingerprint string
	Sniffing    map[string]any
}

type Inbound struct {
	ID             int64  `json:"id"`
	Remark         string `json:"remark"`
	Port           int    `json:"port"`
	Protocol       string `json:"protocol"`
	Enable         bool   `json:"enable"`
	Settings       any    `json:"settings"`
	StreamSettings any    `json:"streamSettings"`
	Sniffing       any    `json:"sniffing"`
}

func object(v any) (map[string]any, error) {
	switch x := v.(type) {
	case map[string]any:
		return x, nil
	case string:
		var m map[string]any
		if json.Unmarshal([]byte(x), &m) != nil {
			return nil, ErrMismatch
		}
		return m, nil
	default:
		b, e := json.Marshal(x)
		if e != nil {
			return nil, ErrMismatch
		}
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return nil, ErrMismatch
		}
		return m, nil
	}
}

func Validate(in Inbound, w Expected) error {
	if in.ID != w.RemoteID || in.Remark != w.Remark || in.Port != w.Port || in.Protocol != "vless" || !in.Enable {
		return ErrMismatch
	}
	s, e := object(in.Settings)
	if e != nil {
		return e
	}
	if s["decryption"] != "none" || s["encryption"] != "none" {
		return fmt.Errorf("%w: vless crypto", ErrMismatch)
	}
	clients, ok := s["clients"].([]any)
	if !ok || len(clients) == 0 {
		return fmt.Errorf("%w: clients", ErrMismatch)
	}
	matched := 0
	for _, rawClient := range clients {
		c, err := object(rawClient)
		if err != nil {
			continue
		}
		if c["id"] == w.UUID && c["flow"] == w.Flow {
			matched++
		}
	}
	if matched != 1 {
		return fmt.Errorf("%w: client contract", ErrMismatch)
	}
	st, e := object(in.StreamSettings)
	if e != nil {
		return e
	}
	if st["network"] != "tcp" || st["security"] != "reality" {
		return fmt.Errorf("%w: stream contract", ErrMismatch)
	}
	rs, e := object(st["realitySettings"])
	if e != nil {
		return e
	}
	if rs["dest"] != w.Target || rs["privateKey"] != w.PrivateKey {
		return fmt.Errorf("%w: reality contract", ErrMismatch)
	}
	var names []string
	b, _ := json.Marshal(rs["serverNames"])
	if json.Unmarshal(b, &names) != nil {
		return fmt.Errorf("%w: server names", ErrMismatch)
	}
	wantNames := append([]string(nil), w.ServerNames...)
	sort.Strings(names)
	sort.Strings(wantNames)
	if len(names) != len(wantNames) {
		return fmt.Errorf("%w: server names", ErrMismatch)
	}
	for i := range names {
		if names[i] != wantNames[i] {
			return fmt.Errorf("%w: server names", ErrMismatch)
		}
	}
	if w.Fingerprint != "" && fmt.Sprint(rs["fingerprint"]) != w.Fingerprint {
		return fmt.Errorf("%w: fingerprint", ErrMismatch)
	}
	if err := validateSniffing(in.Sniffing, w.Sniffing); err != nil {
		return err
	}
	var ids []string
	b, _ = json.Marshal(rs["shortIds"])
	if json.Unmarshal(b, &ids) != nil || len(ids) != 1 || ids[0] != w.ShortID {
		return fmt.Errorf("%w: short id", ErrMismatch)
	}
	return nil
}
