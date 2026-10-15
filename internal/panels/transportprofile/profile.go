// Package transportprofile owns pinned per-listener transport contracts.
// Changing the default affects only new physical listeners.
package transportprofile

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net"
	"strings"
)

const MaxPathBytes = 4096
const ServerMaxHeaderBytes = 32768
const ExtraPreset = `{"xPaddingBytes":"100-1000","noGRPCHeader":true,"scStreamUpServerSecs":"20-80","scMaxEachPostBytes":1000000,"scMinPostsIntervalMs":30,"scMaxConcurrentPosts":4}`
const FinalMaskPreset = `{"tcp":[{"type":"fragment","settings":{"packets":"tlshello","lengths":["48-80","80-128","128-192","192-288","288-448"],"delays":["0-1","1-2","1-2","0-1","0-1"],"maxSplit":"3-5"}}],"udp":[{"type":"noise","settings":{"reset":"90-150","noise":[{"type":"exp","packet":"<t><c><r 12-24>","delay":"0-1"},{"type":"exp","packet":"<n><rc 8-16><rd 4-8>","delay":"0-2"},{"rand":"12-32","randRange":"0-255","delay":"0-1"}]}}],"quicParams":{"congestion":"bbr","bbrProfile":"standard","debug":false,"brutalUp":0,"brutalDown":0,"initStreamReceiveWindow":8388608,"maxStreamReceiveWindow":16777216,"initConnectionReceiveWindow":20971520,"maxConnectionReceiveWindow":41943040,"maxIdleTimeout":45,"keepAlivePeriod":15,"disablePathMTUDiscovery":false,"maxIncomingStreams":512}}`
const FinalMaskStatus = "Requested preset saved; not enabled: Xray 26.9.9 rejects numeric brutalUp/brutalDown and UDP exp. Client compatibility has not been verified."

var ErrInvalid = errors.New("invalid Reality transport profile")
var ErrPinned = errors.New("observed listener transport differs from pinned profile; automatic transport replacement denied")

type Config struct {
	Network          string          `json:"network"`
	Mode             string          `json:"mode,omitempty"`
	Host             string          `json:"host,omitempty"`
	Path             string          `json:"path,omitempty"`
	PathBytes        int             `json:"path_bytes,omitempty"`
	Extra            json.RawMessage `json:"extra,omitempty"`
	FinalMask        json.RawMessage `json:"finalmask,omitempty"`
	FinalMaskEnabled bool            `json:"finalmask_enabled"`
	BrowserDialer    string          `json:"browser_dialer,omitempty"`
	Flow             string          `json:"flow,omitempty"`
}

func Preset() Config {
	return Config{Network: "xhttp", Mode: "stream-up", Host: "apart.com", PathBytes: MaxPathBytes, Extra: json.RawMessage(ExtraPreset), FinalMask: json.RawMessage(FinalMaskPreset)}
}
func (c Config) NetworkName() string {
	if c.Network == "" {
		return "tcp"
	}
	return c.Network
}
func Supported(network string) bool { return network == "tcp" || network == "xhttp" }
func Flow(network string) string {
	if network == "xhttp" {
		return ""
	}
	return "xtls-rprx-vision"
}
func (c Config) Validate(pinned bool) error {
	if c.NetworkName() == "tcp" {
		if c.Mode != "" || c.Host != "" || c.Path != "" || c.PathBytes != 0 || len(c.Extra) > 0 || len(c.FinalMask) > 0 || c.FinalMaskEnabled || c.BrowserDialer != "" || c.Flow != "" {
			return ErrInvalid
		}
		return nil
	}
	if c.Network != "xhttp" || c.Mode != "stream-up" || c.Flow != "" || c.BrowserDialer != "" || c.FinalMaskEnabled {
		return ErrInvalid
	}
	if c.Host != "apart.com" || !validHost(c.Host) || c.PathBytes < 256 || c.PathBytes > MaxPathBytes {
		return ErrInvalid
	}
	if !equalJSON(c.Extra, json.RawMessage(ExtraPreset)) || !equalJSON(c.FinalMask, json.RawMessage(FinalMaskPreset)) {
		return ErrInvalid
	}
	if pinned {
		return ValidatePath(c.Path, c.PathBytes)
	}
	if c.Path != "" {
		return ErrInvalid
	}
	return nil
}
func validHost(h string) bool {
	if len(h) > 253 || h == "" || net.ParseIP(h) != nil {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return strings.Contains(h, ".")
}
func equalJSON(a, b json.RawMessage) bool {
	var x, y any
	dec := json.NewDecoder(bytes.NewReader(a))
	dec.UseNumber()
	if dec.Decode(&x) != nil {
		return false
	}
	dec = json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if dec.Decode(&y) != nil {
		return false
	}
	ax, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return bytes.Equal(ax, by)
}
func ValidatePath(path string, size int) error {
	if size < 256 || size > MaxPathBytes || len(path) != size || !strings.HasPrefix(path, "/") || !strings.HasSuffix(path, "/") {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, s := range strings.Split(strings.Trim(path, "/"), "/") {
		if s == "" || seen[s] {
			return ErrInvalid
		}
		seen[s] = true
		for _, r := range s {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return ErrInvalid
			}
		}
	}
	return nil
}

// NewPath uses each vocabulary segment at most once. The last segment includes
// 128 random bits; the database also enforces uniqueness across listeners.
func NewPath(size int) (string, error) {
	if size < 256 || size > MaxPathBytes {
		return "", ErrInvalid
	}
	words := strings.Fields(vocabulary)
	for i := len(words) - 1; i > 0; i-- {
		j, e := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if e != nil {
			return "", e
		}
		words[i], words[j.Int64()] = words[j.Int64()], words[i]
	}
	nonce := make([]byte, 16)
	if _, e := rand.Read(nonce); e != nil {
		return "", e
	}
	path := "/"
	seen := map[string]bool{}
	for _, w := range words {
		if seen[w] {
			continue
		}
		seen[w] = true
		if len(path)+len(w)+1+33 <= size {
			path += w + "/"
		}
	}
	// Fill any final short remainder with random ASCII. No percent encoding or
	// UTF-8 expansion occurs on the HTTP wire; URI query encoding is separate.
	remaining := size - len(path) - 1
	if remaining < 32 || remaining > 64 {
		return "", errors.New("insufficient unique path vocabulary")
	}
	tail := hex.EncodeToString(nonce)
	if remaining > 32 {
		b := make([]byte, (remaining-32+1)/2)
		if _, e := rand.Read(b); e != nil {
			return "", e
		}
		tail += hex.EncodeToString(b)[:remaining-32]
	}
	path += tail + "/"
	return path, ValidatePath(path, size)
}

// WireSettings never includes client FinalMask or Browser Dialer in the inbound.
func (c Config) WireSettings() map[string]any {
	// XHTTP repeats the URL in legacy Referer padding. A 4096-byte path
	// exceeds the core's default 8192-byte total header budget.
	var extra map[string]json.RawMessage
	_ = json.Unmarshal(c.Extra, &extra)
	extra["serverMaxHeaderBytes"] = json.RawMessage("32768")
	return map[string]any{"host": c.Host, "path": c.Path, "mode": c.Mode, "extra": extra}
}

// FromStream validates only fields needed by client exports; it copies the
// observed Extra object, never substitutes the current default policy.
func FromStream(stream map[string]any) (Config, error) {
	network, _ := stream["network"].(string)
	if network == "tcp" {
		return Config{}, nil
	}
	if network != "xhttp" {
		return Config{}, ErrInvalid
	}
	b, e := json.Marshal(stream["xhttpSettings"])
	if e != nil {
		return Config{}, e
	}
	var x struct {
		Host, Path, Mode string
		Extra            json.RawMessage
	}
	if json.Unmarshal(b, &x) != nil || !validHost(x.Host) || x.Mode != "stream-up" {
		return Config{}, ErrInvalid
	}
	if ValidatePath(x.Path, len(x.Path)) != nil {
		return Config{}, ErrInvalid
	}
	var extra map[string]json.RawMessage
	if json.Unmarshal(x.Extra, &extra) != nil {
		return Config{}, ErrInvalid
	}
	var maxHeader int
	if json.Unmarshal(extra["serverMaxHeaderBytes"], &maxHeader) != nil || maxHeader != ServerMaxHeaderBytes {
		return Config{}, ErrInvalid
	}
	delete(extra, "serverMaxHeaderBytes")
	x.Extra, _ = json.Marshal(extra)
	if !equalJSON(x.Extra, json.RawMessage(ExtraPreset)) {
		return Config{}, ErrInvalid
	}
	return Config{Network: "xhttp", Host: x.Host, Path: x.Path, PathBytes: len(x.Path), Mode: x.Mode, Extra: x.Extra, FinalMask: json.RawMessage(FinalMaskPreset)}, nil
}
