// Package transportapply changes managed listener contracts only through an
// explicit durable request, fresh native readback and verified pin promotion.
package transportapply

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
)

var ErrConflict = errors.New("transport apply contract conflict")

func object(v any) (map[string]any, error) {
	var raw []byte
	var err error
	switch x := v.(type) {
	case string:
		raw = []byte(x)
	case json.RawMessage:
		raw = x
	default:
		raw, err = json.Marshal(v)
	}
	if err != nil {
		return nil, ErrConflict
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var m map[string]any
	if d.Decode(&m) != nil || m == nil {
		return nil, ErrConflict
	}
	return m, nil
}
func decode(raw []byte) (map[string]any, error)     { return object(json.RawMessage(raw)) }
func stringField(m map[string]any, k string) string { v, _ := m[k].(string); return v }
func jsonEqual(a, b any) bool {
	x, e := json.Marshal(a)
	if e != nil {
		return false
	}
	y, e := json.Marshal(b)
	if e != nil {
		return false
	}
	var aa, bb any
	da := json.NewDecoder(bytes.NewReader(x))
	da.UseNumber()
	db := json.NewDecoder(bytes.NewReader(y))
	db.UseNumber()
	return da.Decode(&aa) == nil && db.Decode(&bb) == nil && reflect.DeepEqual(aa, bb)
}
func clients(settings map[string]any) ([]any, error) {
	c, ok := settings["clients"].([]any)
	if !ok {
		return nil, ErrConflict
	}
	seen := map[string]bool{}
	for _, v := range c {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, ErrConflict
		}
		id := stringField(m, "id")
		if id == "" || seen[id] {
			return nil, ErrConflict
		}
		seen[id] = true
	}
	return c, nil
}

// Transform preserves credentials, client identifiers/limits and unrelated
// inbound fields. Only the selected transport and effective flow are changed.
func Transform(raw []byte, key string, target transportprofile.Config) ([]byte, realityconfig.Payload, error) {
	m, e := decode(raw)
	if e != nil {
		return nil, realityconfig.Payload{}, e
	}
	if stringField(m, "remark") != key || stringField(m, "protocol") != "vless" || m["enable"] != true {
		return nil, realityconfig.Payload{}, ErrConflict
	}
	stream, e := object(m["streamSettings"])
	if e != nil {
		return nil, realityconfig.Payload{}, e
	}
	if stringField(stream, "security") != "reality" || !transportprofile.Supported(stringField(stream, "network")) {
		return nil, realityconfig.Payload{}, ErrConflict
	}
	if e = target.Validate(target.NetworkName() == "xhttp"); e != nil {
		return nil, realityconfig.Payload{}, e
	}
	settings, e := object(m["settings"])
	if e != nil {
		return nil, realityconfig.Payload{}, e
	}
	cs, e := clients(settings)
	if e != nil {
		return nil, realityconfig.Payload{}, e
	}
	reality, e := object(stream["realitySettings"])
	if e != nil {
		return nil, realityconfig.Payload{}, e
	}
	if !emptyFallbacks(settings["fallbacks"]) {
		return nil, realityconfig.Payload{}, errors.New("managed listener has custom fallbacks")
	}
	flow := target.ServerFlow()
	settings["flow"] = flow
	settings["decryption"] = "none"
	settings["encryption"] = "none"
	if target.NetworkName() == "xhttp" && flow != "" {
		decryption, encryption, e := transportprofile.EncryptionPair(stringField(reality, "privateKey"), key)
		if e != nil {
			return nil, realityconfig.Payload{}, e
		}
		settings["decryption"] = decryption
		settings["encryption"] = encryption
		delete(settings, "fallbacks")
	}
	for _, v := range cs {
		v.(map[string]any)["flow"] = flow
	}
	m["disableFlow"] = flow == ""
	stream["network"] = target.NetworkName()
	if target.NetworkName() == "xhttp" {
		stream["xhttpSettings"] = target.WireSettings()
		delete(stream, "tcpSettings")
		delete(stream, "rawSettings")
	} else {
		delete(stream, "xhttpSettings")
	}
	delete(stream, "finalmask")
	if target.MaskOnServer() {
		mask, e := target.TCPMask()
		if e != nil {
			return nil, realityconfig.Payload{}, e
		}
		stream["finalmask"] = json.RawMessage(mask)
	}
	m["settings"] = settings
	m["streamSettings"] = stream
	for _, k := range []string{"sniffing", "allocate"} {
		if v, ok := m[k]; ok {
			parsed, err := object(v)
			if err != nil {
				return nil, realityconfig.Payload{}, err
			}
			m[k] = parsed
		}
	}
	// Native form fields are JSON strings, including on older Sanaei versions.
	data, e := json.Marshal(m)
	if e != nil {
		return nil, realityconfig.Payload{}, e
	}
	var payload realityconfig.Payload
	if json.Unmarshal(data, &payload) != nil {
		return nil, payload, ErrConflict
	}
	for _, k := range []string{"settings", "streamSettings", "sniffing", "allocate"} {
		if v, ok := m[k]; ok {
			b, e := json.Marshal(v)
			if e != nil {
				return nil, payload, e
			}
			if _, isText := v.(string); !isText {
				m[k] = string(b)
			}
		}
	}
	data, e = json.Marshal(m)
	return data, payload, e
}
func clientInvariant(s map[string]any) (map[string]any, error) {
	cs, e := clients(s)
	if e != nil {
		return nil, e
	}
	out := map[string]any{}
	for _, v := range cs {
		c := v.(map[string]any)
		copy := map[string]any{}
		for k, x := range c {
			if k != "flow" && k != "created_at" && k != "updated_at" {
				copy[k] = x
			}
		}
		out[stringField(c, "id")] = copy
	}
	return out, nil
}

// Verify compares protected fields and the native transport, accepting the
// v3 API's stored client flow when settings.flow supplies the effective flow.
func Verify(before, after []byte, key string, target transportprofile.Config) error {
	expected, _, e := Transform(before, key, target)
	if e != nil {
		return e
	}
	a, e := decode(expected)
	if e != nil {
		return e
	}
	b, e := decode(after)
	if e != nil {
		return e
	}
	// Native counters and generated tag may change while saving. All other
	// unrelated fields, including unknown extensions, must survive.
	volatile := map[string]bool{"settings": true, "streamSettings": true, "disableFlow": true, "up": true, "down": true, "clientStats": true, "tag": true, "updated_at": true, "created_at": true, "lastTrafficResetTime": true}
	for _, k := range unionKeys(a, b) {
		if volatile[k] {
			continue
		}
		av, bv := a[k], b[k]
		if k == "sniffing" || k == "allocate" {
			av, _ = object(av)
			bv, _ = object(bv)
		}
		if !jsonEqual(av, bv) {
			return fmt.Errorf("%w: protected inbound field %s", ErrConflict, k)
		}
	}
	sa, e := object(a["settings"])
	if e != nil {
		return e
	}
	sb, e := object(b["settings"])
	if e != nil {
		return e
	}
	ca, e := clientInvariant(sa)
	if e != nil {
		return e
	}
	cb, e := clientInvariant(sb)
	if e != nil || !jsonEqual(ca, cb) {
		return fmt.Errorf("%w: client identity or limits changed", ErrConflict)
	}
	actualFlow, e := transportprofile.InboundServerFlow(target.NetworkName(), sb)
	if e != nil || actualFlow != target.ServerFlow() {
		return fmt.Errorf("%w: native flow", ErrConflict)
	}
	for _, k := range []string{"decryption", "encryption"} {
		av, bv := stringField(sa, k), stringField(sb, k)
		if av == "" {
			av = "none"
		}
		if bv == "" {
			bv = "none"
		}
		if av != bv {
			return fmt.Errorf("%w: native %s", ErrConflict, k)
		}
	}
	for _, k := range unionKeys(sa, sb) {
		if k == "clients" || k == "flow" || k == "decryption" || k == "encryption" {
			continue
		}
		if k == "fallbacks" {
			if !emptyFallbacks(sa[k]) || !emptyFallbacks(sb[k]) {
				return fmt.Errorf("%w: native fallbacks", ErrConflict)
			}
			continue
		}
		if !jsonEqual(sa[k], sb[k]) {
			return fmt.Errorf("%w: unrelated settings %s", ErrConflict, k)
		}
	}
	actualClients, _ := clients(sb)
	for _, v := range actualClients {
		flow := stringField(v.(map[string]any), "flow")
		if flow != "" && flow != target.ServerFlow() {
			return fmt.Errorf("%w: client flow", ErrConflict)
		}
	}
	as, e := object(a["streamSettings"])
	if e != nil {
		return e
	}
	bs, e := object(b["streamSettings"])
	if e != nil {
		return e
	}
	for _, k := range unionKeys(as, bs) {
		if k == "xhttpSettings" || k == "tcpSettings" || k == "rawSettings" {
			continue
		}
		if !jsonEqual(as[k], bs[k]) {
			return fmt.Errorf("%w: stream %s", ErrConflict, k)
		}
	}
	if target.NetworkName() == "xhttp" {
		observed, e := transportprofile.FromStream(bs)
		if e != nil {
			return e
		}
		if _, e = transportprofile.ApplyPinnedClientOptions(observed, target); e != nil {
			return e
		}
	}
	return nil
}
func digest(v string) string { x := sha256.Sum256([]byte(v)); return hex.EncodeToString(x[:]) }

func unionKeys(a, b map[string]any) []string {
	seen := map[string]bool{}
	out := []string{}
	for k := range a {
		seen[k] = true
		out = append(out, k)
	}
	for k := range b {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out
}

func emptyFallbacks(v any) bool {
	if v == nil {
		return true
	}
	a, ok := v.([]any)
	return ok && len(a) == 0
}
