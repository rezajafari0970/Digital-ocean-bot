// Package ipwire defines pure provider wire contracts. It performs no IO and
// grants no allocation, guest-network or cleanup authority.
package ipwire

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrUnsupported = errors.New("IP product or action not supported by wire contract")
var ErrSpec = errors.New("incomplete or invalid explicit IP wire specification")
var segment = regexp.MustCompile("^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$")
var uuid = regexp.MustCompile("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")

// Spec must be derived from approved durable evidence by the caller. Values
// here are not proof of quota, funds, interface compatibility or user consent.
type Spec struct {
	Provider                          string
	Family                            int
	Product, Action, Region, ServerID string
	PrefixLength                      int
	InterfaceMode, ReleasePolicy      string
}

// Prepared binds wire bytes AND intended scope. Detached allocation still has
// one selected target server. Callers cannot mutate the internal wire bytes.
type Prepared struct {
	spec        Spec
	method, url string
	body        []byte
	impact      string
}

func (p Prepared) Spec() Spec     { return p.spec }
func (p Prepared) Method() string { return p.method }
func (p Prepared) URL() string    { return p.url }
func (p Prepared) Body() []byte   { return bytes.Clone(p.body) }
func (p Prepared) Impact() string { return p.impact }
func (p Prepared) Digest() string {
	b, _ := json.Marshal(struct {
		Version     string
		Spec        Spec
		Method, URL string
		Body        json.RawMessage
		Impact      string
	}{"ip-wire/v1", p.spec, p.method, p.url, p.body, p.impact})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func positiveID(s string) bool {
	n, e := strconv.ParseInt(s, 10, 64)
	return e == nil && n > 0 && strconv.FormatInt(n, 10) == s
}
func address(s string, f int) (string, error) {
	a, e := netip.ParseAddr(s)
	if e != nil || a.Zone() != "" || a.Is4In6() || a.Is4() != (f == 4) || !a.IsGlobalUnicast() {
		return "", ErrSpec
	}
	return a.String(), nil
}
func prefix(s string, f, bits int) (string, error) {
	p, e := netip.ParsePrefix(s)
	if e != nil || p.Addr().Is4In6() || p.Addr().Is4() != (f == 4) || !p.Addr().IsGlobalUnicast() || p != p.Masked() || p.Bits() != bits {
		return "", ErrSpec
	}
	return p.String(), nil
}
func Prepare(s Spec) (Prepared, error) {
	if s.Action != "ALLOCATE" {
		return Prepared{}, ErrUnsupported
	}
	if (s.Family != 4 && s.Family != 6) || !segment.MatchString(s.Region) || s.ServerID == "" {
		return Prepared{}, ErrSpec
	}
	p := Prepared{spec: s, method: http.MethodPost, impact: "DETACHED_ALLOCATION"}
	var body any
	switch s.Provider {
	case "digitalocean":
		if s.Product != "reserved" {
			return Prepared{}, ErrUnsupported
		}
		if !positiveID(s.ServerID) || s.PrefixLength != 0 || s.InterfaceMode != "" || s.ReleasePolicy != "" {
			return Prepared{}, ErrSpec
		}
		p.url = "https://api.digitalocean.com/v2/reserved_ips"
		body = map[string]any{"region": s.Region}
		if s.Family == 6 {
			p.url = "https://api.digitalocean.com/v2/reserved_ipv6"
			body = map[string]any{"region_slug": s.Region}
		}
	case "vultr":
		if s.Product != "reserved" {
			return Prepared{}, ErrUnsupported
		}
		if !uuid.MatchString(s.ServerID) || s.InterfaceMode != "" || s.ReleasePolicy != "" || (s.Family == 4 && s.PrefixLength != 0) || (s.Family == 6 && s.PrefixLength != 64) {
			return Prepared{}, ErrSpec
		}
		p.url = "https://api.vultr.com/v2/reserved-ips"
		body = map[string]any{"region": s.Region, "ip_type": fmt.Sprintf("v%d", s.Family)}
	case "linode":
		if !positiveID(s.ServerID) || s.ReleasePolicy != "" {
			return Prepared{}, ErrSpec
		}
		id, _ := strconv.ParseInt(s.ServerID, 10, 64)
		p.impact = "ALLOCATION_WITH_ATTACHMENT"
		switch {
		case s.Product == "public" && s.Family == 4:
			if s.PrefixLength != 0 || s.InterfaceMode != "legacy" {
				return Prepared{}, ErrSpec
			}
			p.url = "https://api.linode.com/v4/networking/ips"
			body = map[string]any{"linode_id": id, "type": "ipv4", "public": true}
		case s.Product == "routed" && s.Family == 6:
			if (s.PrefixLength != 56 && s.PrefixLength != 64) || s.InterfaceMode != "legacy" {
				return Prepared{}, ErrSpec
			}
			p.url = "https://api.linode.com/v4/networking/ipv6/ranges"
			body = map[string]any{"linode_id": id, "prefix_length": s.PrefixLength}
		default:
			return Prepared{}, ErrUnsupported
		}
	case "upcloud":
		if !uuid.MatchString(s.ServerID) || s.PrefixLength != 0 || s.InterfaceMode != "" {
			return Prepared{}, ErrSpec
		}
		p.url = "https://api.upcloud.com/1.3/ip_address"
		fields := map[string]any{"family": fmt.Sprintf("IPv%d", s.Family), "access": "public"}
		switch {
		case s.Product == "public":
			p.impact = "ALLOCATION_WITH_ATTACHMENT"
			fields["server"] = s.ServerID
			if s.Family == 4 {
				if s.ReleasePolicy != "keep" && s.ReleasePolicy != "release" {
					return Prepared{}, ErrSpec
				}
				fields["release_policy"] = s.ReleasePolicy
			} else if s.ReleasePolicy != "" {
				return Prepared{}, ErrSpec
			}
		case s.Product == "floating" && s.Family == 4:
			if s.ReleasePolicy != "keep" && s.ReleasePolicy != "release" {
				return Prepared{}, ErrSpec
			}
			fields["floating"] = "yes"
			fields["zone"] = s.Region
			fields["release_policy"] = s.ReleasePolicy
		default:
			return Prepared{}, ErrUnsupported
		}
		body = map[string]any{"ip_address": fields}
	default:
		return Prepared{}, ErrUnsupported
	}
	var e error
	p.body, e = json.Marshal(body)
	return p, e
}

// Result never proves ownership, reachability or terminal absence. ACQUIRED
// still needs durable receipt correlation and fresh inventory confirmation.
// Candidates are untrusted forensic hints only; they must never authorize adoption,
// attachment or cleanup. An UNKNOWN reply can still describe a billable resource.
type Candidate struct {
	ResourceID, Address, Prefix, Region, ServerID string
	Family                                        int
}
type Result struct {
	Candidates                                                          []Candidate
	Outcome                                                             string
	HTTPStatus                                                          int
	ResourceID, Address, Prefix, ServerID, Region, Contract, ErrorClass string
	RetryAt                                                             *time.Time
	BodyDigest                                                          string
}

func unknown(status int, b []byte) Result {
	h := sha256.Sum256(b)
	class := "UNKNOWN_RESULT"
	switch {
	case status == 0:
		class = "NETWORK"
	case status == 429:
		class = "RATE_LIMIT"
	case status == 401 || status == 403:
		class = "AUTH"
	case status == 402:
		class = "PAYMENT"
	case status >= 500:
		class = "PROVIDER_UNAVAILABLE"
	}
	return Result{Outcome: "UNKNOWN", HTTPStatus: status, ErrorClass: class, BodyDigest: hex.EncodeToString(h[:])}
}

// strictJSON allows additional fields but rejects duplicate keys, trailing data,
// and oversized responses. No raw provider error text is exposed.
func strictJSON(b []byte, out any) error {
	if len(b) == 0 || len(b) > 1<<20 {
		return ErrSpec
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 64 {
			return ErrSpec
		}
		t, e := d.Token()
		if e != nil {
			return e
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					k, e := d.Token()
					if e != nil {
						return e
					}
					s, ok := k.(string)
					// encoding/json Unicode folds field names; restrict object keys to
					// ASCII so duplicate equivalence matches its ASCII case folding.
					for _, r := range s {
						if r < 32 || r > 126 {
							return ErrSpec
						}
					}
					if !ok || seen[strings.ToLower(s)] {
						return ErrSpec
					}
					seen[strings.ToLower(s)] = true
					if e = value(depth + 1); e != nil {
						return e
					}
				}
				t, e = d.Token()
				if e != nil || t != json.Delim('}') {
					return ErrSpec
				}
			case '[':
				for d.More() {
					if e = value(depth + 1); e != nil {
						return e
					}
				}
				t, e = d.Token()
				if e != nil || t != json.Delim(']') {
					return ErrSpec
				}
			default:
				return ErrSpec
			}
		}
		return nil
	}
	if e := value(0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrSpec
	}
	return json.Unmarshal(b, out)
}
func (p Prepared) Decode(status int, b []byte, headers http.Header, now time.Time) Result {
	out := unknown(status, b)
	out.Candidates = p.candidates(b)
	if status == 429 {
		if v := headers.Get("Retry-After"); v != "" {
			if secs, e := strconv.ParseInt(v, 10, 32); e == nil && secs >= 0 {
				t := now.Add(time.Duration(secs) * time.Second)
				out.RetryAt = &t
			} else if t, e := http.ParseTime(v); e == nil && t.After(now) {
				out.RetryAt = &t
			}
		}
	}
	want := 201
	if p.spec.Provider == "linode" {
		want = 200
	}
	if p.spec.Provider == "digitalocean" && p.spec.Family == 4 {
		want = 202
	}
	if p.method != http.MethodPost || p.spec.Action != "ALLOCATE" || status != want {
		return out
	}
	resource, e := p.created(b)
	if e != nil {
		out.ErrorClass = "INVALID_PROVIDER_RESPONSE"
		return out
	}
	out.ResourceID = resource.ResourceID
	out.Address = resource.Address
	out.Prefix = resource.Prefix
	out.ServerID = resource.ServerID
	out.Region = resource.Region
	out.Outcome = "ACQUIRED"
	out.ErrorClass = ""
	out.Contract = fmt.Sprintf("%s/%s/ipv%d/ALLOCATE/created-%d/v1", p.spec.Provider, p.spec.Product, p.spec.Family, want)
	if p.spec.Provider == "digitalocean" && p.spec.Family == 4 {
		out.Outcome = "ACKNOWLEDGED"
		out.Contract = ""
	}
	return out
}
