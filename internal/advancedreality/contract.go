// Package advancedreality provides experimental, non-actuating control-plane
// tools. It has no panel, network, shell, or production configuration writer.
package advancedreality

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxConfigBytes = 1 << 20
const Version = "0.1.0-experimental"

// Errors intentionally contain no parser excerpts, configuration values or
// untrusted JSON paths: upstream validators can echo private keys on failure.
var (
	ErrJSON      = errors.New("invalid_or_ambiguous_json")
	ErrShape     = errors.New("configuration_structure_changed")
	ErrImmutable = errors.New("value_change_not_allowlisted")
	ErrVersion   = errors.New("core_version_not_audited")
	ErrRange     = errors.New("value_outside_experimental_bounds")
)

type Plan struct {
	Version            string   `json:"version"`
	CoreVersion        string   `json:"core_version"`
	BaselineSHA256     string   `json:"baseline_sha256"`
	CandidateSHA256    string   `json:"candidate_sha256"`
	ChangedPaths       []string `json:"changed_paths"`
	StructurePreserved bool     `json:"structure_preserved"`
	CanApply           bool     `json:"can_apply"`
	Blockers           []string `json:"blockers"`
}

func Digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func parse(raw []byte) (any, error) {
	if len(raw) == 0 || len(raw) > MaxConfigBytes || !utf8.Valid(raw) {
		return nil, ErrJSON
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	nodes := 0
	v, err := value(d, 0, &nodes)
	if err != nil {
		return nil, ErrJSON
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, ErrJSON
	}
	if _, ok := v.(map[string]any); !ok {
		return nil, ErrJSON
	}
	return v, nil
}

func value(d *json.Decoder, depth int, nodes *int) (any, error) {
	*nodes++
	if depth > 64 || *nodes > 100000 {
		return nil, ErrJSON
	}
	t, e := d.Token()
	if e != nil {
		return nil, e
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return t, nil
	}
	switch delim {
	case '{':
		m := map[string]any{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return nil, e
			}
			key, ok := k.(string)
			if !ok {
				return nil, ErrJSON
			}
			if _, exists := m[key]; exists {
				return nil, ErrJSON
			}
			v, e := value(d, depth+1, nodes)
			if e != nil {
				return nil, e
			}
			m[key] = v
		}
		t, e = d.Token()
		if e != nil || t != json.Delim('}') {
			return nil, ErrJSON
		}
		return m, nil
	case '[':
		a := []any{}
		for d.More() {
			v, e := value(d, depth+1, nodes)
			if e != nil {
				return nil, e
			}
			a = append(a, v)
		}
		t, e = d.Token()
		if e != nil || t != json.Delim(']') {
			return nil, ErrJSON
		}
		return a, nil
	}
	return nil, ErrJSON
}

// Inspect compares parsed JSON, preserving keys, nesting, JSON types and array
// length/order. Unknown fields remain immutable. This is not Xray validation.
func Inspect(before, after []byte, core string) (Plan, error) {
	p := Plan{Version: Version, CoreVersion: core, BaselineSHA256: Digest(before), CandidateSHA256: Digest(after), ChangedPaths: []string{}, Blockers: []string{"core_native_validation_required", "official_client_compatibility_required", "canary_health_required", "actuator_not_installed"}}
	// Exact audited releases only. No semver guessing of experimental support.
	switch core {
	case "26.3.27", "26.5.9", "26.9.9", "26.10.10":
	default:
		return Plan{}, ErrVersion
	}
	a, e := parse(before)
	if e != nil {
		return Plan{}, e
	}
	b, e := parse(after)
	if e != nil {
		return Plan{}, e
	}
	if e = rejectRealityInboundMask(a); e != nil {
		return Plan{}, e
	}
	if e = rejectRealityInboundMask(b); e != nil {
		return Plan{}, e
	}
	if e = compare(a, b, "", &p.ChangedPaths); e != nil {
		return Plan{}, e
	}
	sort.Strings(p.ChangedPaths)
	p.StructurePreserved = true
	return p, nil
}

func compare(a, b any, path string, changed *[]string) error {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return ErrShape
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := x[k]
			z, exists := y[k]
			if !exists {
				return ErrShape
			}
			if e := compare(v, z, path+"/"+strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1"), changed); e != nil {
				return e
			}
		}
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return ErrShape
		}
		for i := range x {
			if e := compare(x[i], y[i], path+"/"+strconv.Itoa(i), changed); e != nil {
				return e
			}
		}
	case json.Number:
		y, ok := b.(json.Number)
		if !ok {
			return ErrShape
		}
		if x == y {
			return nil
		}
		if !allowedNumeric(path) {
			return ErrImmutable
		}
		n, e := y.Int64()
		if e != nil {
			return ErrRange
		}
		if strings.HasSuffix(path, "/bufferSize") {
			if n < 16 || n > 512 {
				return ErrRange
			}
		} else if n < 4 || n > 30 {
			return ErrRange
		}
		*changed = append(*changed, path)
	case string:
		y, ok := b.(string)
		if !ok {
			return ErrShape
		}
		if x != y {
			return ErrImmutable
		}
	case bool:
		y, ok := b.(bool)
		if !ok {
			return ErrShape
		}
		if x != y {
			return ErrImmutable
		}
	case nil:
		if b != nil {
			return ErrShape
		}
	default:
		return ErrJSON
	}
	return nil
}

// Initial server-only proposals. bufferSize is KiB per internal request pipe,
// not a cap on TLS/HTTP/socket or inbound memory. handshake is a policy-level
// timeout, not a universal TLS handshake deadline. Neither changes on a server
// until a separately reviewed actuator and real client tests exist.
func allowedNumeric(path string) bool {
	p := strings.Split(path, "/")
	if len(p) != 5 || p[1] != "policy" || p[2] != "levels" || (p[4] != "bufferSize" && p[4] != "handshake") {
		return false
	}
	n, e := strconv.ParseUint(p[3], 10, 32)
	return e == nil && strconv.FormatUint(n, 10) == p[3]
}
