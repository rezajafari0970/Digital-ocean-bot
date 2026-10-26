package transportprofile

import (
	"bytes"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const Vision = "xtls-rprx-vision"
const VisionUDP443 = Vision + "-udp443"
const ActiveFinalMaskPreset = `{"tcp":[{"type":"fragment","settings":{"packets":"tlshello","lengths":["48-80","80-128","128-192","192-288","288-448"],"delays":["0-1","1-2","1-2","0-1","0-1"],"maxSplit":"3-5"}}]}`

func Recommended() Config {
	c := Preset()
	c.Flow = VisionUDP443
	c.FinalMask = json.RawMessage(ActiveFinalMaskPreset)
	c.FinalMaskEnabled = true
	c.FinalMaskScope = "both"
	return c
}
func (c Config) ServerFlow() string {
	if c.NetworkName() == "tcp" || c.Flow == Vision || c.Flow == VisionUDP443 {
		return Vision
	}
	return ""
}
func (c Config) MaskOnServer() bool { return c.FinalMaskEnabled && c.FinalMaskScope != "client" }
func (c Config) MaskOnClient() bool { return c.FinalMaskEnabled && c.FinalMaskScope != "server" }

// Browser Dialer is an application-level preference. REALITY always uses native
// dialing; an ordinary VLESS URI cannot enable v2rayNG's browserDialerMode.
func (c Config) BrowserDialerStatus() string {
	if c.BrowserDialer == "" {
		return "Disabled"
	}
	return "Requested " + c.BrowserDialer + "; not active with REALITY. Requires a TLS endpoint and activation in the client app."
}
func jsonObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if len(raw) > 32768 || json.Unmarshal(raw, &m) != nil || m == nil {
		return nil, ErrInvalid
	}
	return m, nil
}
func integer(raw json.RawMessage, min, max int64) bool {
	var n int64
	return !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && json.Unmarshal(raw, &n) == nil && n >= min && n <= max
}
func rangeValue(raw json.RawMessage, min, max int64) bool {
	if integer(raw, min, max) {
		return true
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return false
	}
	parts := strings.Split(s, "-")
	if len(parts) < 1 || len(parts) > 2 {
		return false
	}
	first, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || first < min || first > max {
		return false
	}
	last := first
	if len(parts) == 2 {
		last, e = strconv.ParseInt(parts[1], 10, 64)
	}
	return e == nil && last >= first && last <= max
}
func ValidateExtra(raw json.RawMessage) error {
	m, e := jsonObject(raw)
	if e != nil {
		return fmt.Errorf("%w: XHTTP Extra must be a JSON object", ErrInvalid)
	}
	ranges := " xPaddingBytes sessionIDLength uplinkChunkSize scMaxEachPostBytes scMinPostsIntervalMs scStreamUpServerSecs "
	booleans := " xPaddingObfsMode noGRPCHeader noSSEHeader "
	texts := " xPaddingKey xPaddingHeader xPaddingPlacement xPaddingMethod uplinkHTTPMethod sessionIDPlacement sessionIDKey sessionIDTable seqPlacement seqKey uplinkDataPlacement uplinkDataKey "
	for k, v := range m {
		valid := false
		switch {
		case strings.Contains(ranges, " "+k+" "):
			valid = rangeValue(v, 0, 2147483647)
		case strings.Contains(booleans, " "+k+" "):
			var b bool
			valid = json.Unmarshal(v, &b) == nil && string(v) != "null"
		case strings.Contains(texts, " "+k+" "):
			var s string
			valid = json.Unmarshal(v, &s) == nil && len(s) <= 512 && !strings.ContainsAny(s, "\r\n\x00")
		case k == "scMaxBufferedPosts" || k == "scMaxConcurrentPosts":
			valid = integer(v, 0, 65536)
		case k == "serverMaxHeaderBytes":
			valid = integer(v, 16384, 1048576)
		case k == "headers":
			var headers map[string]string
			valid = json.Unmarshal(v, &headers) == nil && headers != nil
			for name, value := range headers {
				if strings.EqualFold(name, "host") || strings.ContainsAny(name+value, "\r\n\x00") {
					valid = false
				}
			}
		case k == "xmux" || k == "downloadSettings":
			_, err := jsonObject(v)
			valid = err == nil
		}
		if !valid {
			return fmt.Errorf("%w: invalid or unknown XHTTP Extra field %q", ErrInvalid, k)
		}
	}
	return nil
}

// TCPMask is the effective mask on HTTP/2. Saved UDP/QUIC settings are never
// exported as active on a TCP transport, and cannot break older client cores.
func (c Config) TCPMask() (json.RawMessage, error) {
	m, e := jsonObject(c.FinalMask)
	if e != nil {
		return nil, fmt.Errorf("%w: invalid FinalMask object", ErrInvalid)
	}
	for k := range m {
		if k != "tcp" && k != "udp" && k != "quicParams" {
			return nil, fmt.Errorf("%w: unknown FinalMask field %q", ErrInvalid, k)
		}
	}
	var masks []struct {
		Type     string          `json:"type"`
		Settings json.RawMessage `json:"settings"`
	}
	dec := json.NewDecoder(bytes.NewReader(m["tcp"]))
	dec.DisallowUnknownFields()
	if dec.Decode(&masks) != nil || len(masks) < 1 || len(masks) > 8 {
		return nil, fmt.Errorf("%w: FinalMask requires 1–8 TCP fragment masks", ErrInvalid)
	}
	for _, mask := range masks {
		if mask.Type != "fragment" {
			return nil, fmt.Errorf("%w: this REALITY core supports the fragment TCP mask", ErrInvalid)
		}
		settings, e := jsonObject(mask.Settings)
		if e != nil {
			return nil, ErrInvalid
		}
		if len(settings) == 0 {
			return nil, ErrInvalid
		}
		for k, v := range settings {
			valid := false
			switch k {
			case "packets":
				var s string
				valid = json.Unmarshal(v, &s) == nil && (s == "tlshello" || rangeValue(v, 1, 65535))
			case "lengths", "delays":
				var values []json.RawMessage
				valid = json.Unmarshal(v, &values) == nil && len(values) > 0 && len(values) <= 64
				for _, x := range values {
					min, max := int64(1), int64(16384)
					if k == "delays" {
						min, max = 0, 1000
					}
					if !rangeValue(x, min, max) {
						valid = false
					}
				}
			case "maxSplit":
				valid = rangeValue(v, 0, 64)
			case "length":
				valid = rangeValue(v, 1, 16384)
			case "delay":
				valid = rangeValue(v, 0, 1000)
			}
			if !valid {
				return nil, fmt.Errorf("%w: invalid fragment setting %q", ErrInvalid, k)
			}
		}
		if _, ok := settings["packets"]; !ok {
			return nil, fmt.Errorf("%w: missing fragment packets", ErrInvalid)
		}
		for _, pair := range [][2]string{{"length", "lengths"}, {"delay", "delays"}} {
			if _, scalar := settings[pair[0]]; !scalar {
				if _, schedule := settings[pair[1]]; !schedule {
					return nil, fmt.Errorf("%w: missing fragment %s or %s", ErrInvalid, pair[0], pair[1])
				}
			}
		}
	}
	b, e := json.Marshal(map[string]json.RawMessage{"tcp": m["tcp"]})
	return b, e
}

// EncryptionPair derives a domain-separated authentication key from the
// existing encrypted listener secret. It never reuses the raw REALITY key.
// This is deterministic across retries and requires no plaintext key storage.
func EncryptionPair(realityPrivate, listener string) (decryption, encryption string, err error) {
	key, e := base64.RawURLEncoding.DecodeString(realityPrivate)
	if e != nil || len(key) != 32 || listener == "" {
		return "", "", ErrInvalid
	}
	defer clear(key)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("digital-ocean-bot/vless-encryption/v1\x00"))
	mac.Write([]byte(listener))
	derived := mac.Sum(nil)
	defer clear(derived)
	private, e := ecdh.X25519().NewPrivateKey(derived)
	if e != nil {
		return "", "", ErrInvalid
	}
	return "mlkem768x25519plus.native.600s." + base64.RawURLEncoding.EncodeToString(derived), "mlkem768x25519plus.native.0rtt." + base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()), nil
}
func ClientEncryption(decryption string) (string, error) {
	if decryption == "none" || decryption == "" {
		return "none", nil
	}
	parts := strings.Split(decryption, ".")
	if len(parts) != 4 || parts[0] != "mlkem768x25519plus" || parts[1] != "native" || parts[2] != "600s" {
		return "", ErrInvalid
	}
	key, e := base64.RawURLEncoding.DecodeString(parts[3])
	if e != nil || len(key) != 32 {
		return "", ErrInvalid
	}
	defer clear(key)
	private, e := ecdh.X25519().NewPrivateKey(key)
	if e != nil {
		return "", ErrInvalid
	}
	return "mlkem768x25519plus.native.0rtt." + base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()), nil
}
func ValidClientEncryption(value string) bool {
	p := strings.Split(value, ".")
	if len(p) != 4 || p[0] != "mlkem768x25519plus" || p[1] != "native" || p[2] != "0rtt" {
		return false
	}
	b, e := base64.RawURLEncoding.DecodeString(p[3])
	return e == nil && len(b) == 32
}

// InboundServerFlow reads the actual server contract, never a new default.
func InboundServerFlow(network string, settings map[string]any) (string, error) {
	if network == "tcp" {
		return Vision, nil
	}
	if network != "xhttp" {
		return "", ErrInvalid
	}
	flow, _ := settings["flow"].(string)
	if flow != "" && flow != Vision {
		return "", ErrInvalid
	}
	if flow == "" {
		if clients, ok := settings["clients"].([]any); ok {
			for _, v := range clients {
				if c, ok := v.(map[string]any); ok {
					f, _ := c["flow"].(string)
					if f != "" {
						if f != Vision {
							return "", ErrInvalid
						}
						flow = f
					}
				}
			}
		}
	}
	if flow != "" {
		decryption, _ := settings["decryption"].(string)
		enc, e := ClientEncryption(decryption)
		if e != nil || enc == "none" {
			return "", fmt.Errorf("%w: XHTTP Vision requires VLESS Encryption", ErrInvalid)
		}
	}
	return flow, nil
}
