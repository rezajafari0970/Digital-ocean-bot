package ipwire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

const server = "11111111-2222-3333-4444-555555555555"
const resource = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

type wireCase struct {
	name                                  string
	spec                                  Spec
	url, body, reply, outcome, id, impact string
	status                                int
}

func cases() []wireCase {
	return []wireCase{
		{"do4", Spec{Provider: "digitalocean", Family: 4, Product: "reserved", Action: "ALLOCATE", Region: "nyc3", ServerID: "123"}, "https://api.digitalocean.com/v2/reserved_ips", `{"region":"nyc3"}`, `{"reserved_ip":{"ip":"203.0.113.7","region":{"slug":"nyc3"},"droplet":null}}`, "ACKNOWLEDGED", "203.0.113.7", "DETACHED_ALLOCATION", 202},
		{"do6", Spec{Provider: "digitalocean", Family: 6, Product: "reserved", Action: "ALLOCATE", Region: "nyc3", ServerID: "123"}, "https://api.digitalocean.com/v2/reserved_ipv6", `{"region_slug":"nyc3"}`, `{"reserved_ipv6":{"ip":"2001:0DB8:0001::7","region_slug":"nyc3","droplet":null}}`, "ACQUIRED", "2001:db8:1::7", "DETACHED_ALLOCATION", 201},
		{"vultr4", Spec{Provider: "vultr", Family: 4, Product: "reserved", Action: "ALLOCATE", Region: "ewr", ServerID: server}, "https://api.vultr.com/v2/reserved-ips", `{"ip_type":"v4","region":"ewr"}`, `{"reserved_ip":{"id":"` + resource + `","region":"ewr","ip_type":"v4","subnet":"203.0.113.8","subnet_size":32,"instance_id":""}}`, "ACQUIRED", resource, "DETACHED_ALLOCATION", 201},
		{"vultr6", Spec{Provider: "vultr", Family: 6, Product: "reserved", Action: "ALLOCATE", Region: "ewr", ServerID: server, PrefixLength: 64}, "https://api.vultr.com/v2/reserved-ips", `{"ip_type":"v6","region":"ewr"}`, `{"reserved_ip":{"id":"` + resource + `","region":"ewr","ip_type":"v6","subnet":"2001:db8:2::","subnet_size":64,"instance_id":""}}`, "ACQUIRED", resource, "DETACHED_ALLOCATION", 201},
		{"linode4", Spec{Provider: "linode", Family: 4, Product: "public", Action: "ALLOCATE", Region: "us-east", ServerID: "123", InterfaceMode: "legacy"}, "https://api.linode.com/v4/networking/ips", `{"linode_id":123,"public":true,"type":"ipv4"}`, `{"address":"203.0.113.9","type":"ipv4","region":"us-east","public":true,"reserved":false,"linode_id":123}`, "ACQUIRED", "203.0.113.9", "ALLOCATION_WITH_ATTACHMENT", 200},
		{"linode56", Spec{Provider: "linode", Family: 6, Product: "routed", Action: "ALLOCATE", Region: "us-east", ServerID: "123", InterfaceMode: "legacy", PrefixLength: 56}, "https://api.linode.com/v4/networking/ipv6/ranges", `{"linode_id":123,"prefix_length":56}`, `{"range":"2001:db8:300::/56","route_target":"2001:db8::1"}`, "ACQUIRED", "2001:db8:300::/56", "ALLOCATION_WITH_ATTACHMENT", 200},
		{"upcloud4", Spec{Provider: "upcloud", Family: 4, Product: "public", Action: "ALLOCATE", Region: "fi-hel2", ServerID: server, ReleasePolicy: "keep"}, "https://api.upcloud.com/1.3/ip_address", `{"ip_address":{"access":"public","family":"IPv4","release_policy":"keep","server":"` + server + `"}}`, `{"ip_address":{"address":"203.0.113.10","access":"public","family":"IPv4","server":"` + server + `","zone":"fi-hel2","floating":"no","release_policy":"keep"}}`, "ACQUIRED", "203.0.113.10", "ALLOCATION_WITH_ATTACHMENT", 201},
		{"upcloud6", Spec{Provider: "upcloud", Family: 6, Product: "public", Action: "ALLOCATE", Region: "fi-hel2", ServerID: server}, "https://api.upcloud.com/1.3/ip_address", `{"ip_address":{"access":"public","family":"IPv6","server":"` + server + `"}}`, `{"ip_address":{"address":"2001:db8:4::7","access":"public","family":"IPv6","server":"` + server + `","zone":"fi-hel2","floating":"no"}}`, "ACQUIRED", "2001:db8:4::7", "ALLOCATION_WITH_ATTACHMENT", 201},
		{"upcloud_floating", Spec{Provider: "upcloud", Family: 4, Product: "floating", Action: "ALLOCATE", Region: "fi-hel2", ServerID: server, ReleasePolicy: "keep"}, "https://api.upcloud.com/1.3/ip_address", `{"ip_address":{"access":"public","family":"IPv4","floating":"yes","release_policy":"keep","zone":"fi-hel2"}}`, `{"ip_address":{"address":"203.0.113.11","access":"public","family":"IPv4","zone":"fi-hel2","floating":"yes","release_policy":"keep"}}`, "ACQUIRED", "203.0.113.11", "DETACHED_ALLOCATION", 201},
	}
}
func TestAllocationContracts(t *testing.T) {
	for _, c := range cases() {
		t.Run(c.name, func(t *testing.T) {
			p, e := Prepare(c.spec)
			if e != nil {
				t.Fatal(e)
			}
			if p.URL() != c.url || p.Method() != http.MethodPost || string(p.Body()) != c.body || p.Impact() != c.impact {
				t.Fatalf("wrong wire plan: %s %s %s", p.URL(), p.Body(), p.Impact())
			}
			b := p.Body()
			b[0] = 'x'
			if string(p.Body()) != c.body {
				t.Fatal("mutable wire body")
			}
			got := p.Decode(c.status, []byte(c.reply), nil, time.Now())
			if got.Outcome != c.outcome || got.ResourceID != c.id || got.ErrorClass != "" {
				t.Fatalf("decode: %+v", got)
			}
			if c.outcome == "ACKNOWLEDGED" && got.Contract != "" {
				t.Fatal("DO202 falsely acquired")
			}
			for _, status := range []int{0, 200, 201, 202, 204, 301, 400, 401, 402, 403, 404, 409, 422, 429, 500, 503} {
				if status == c.status {
					continue
				}
				r := p.Decode(status, []byte(c.reply), nil, time.Now())
				if r.Outcome != "UNKNOWN" || r.ResourceID != "" {
					t.Fatalf("wrong-status success: %+v", r)
				}
			}
			for _, b := range []string{"", "null", "{}", c.reply + "{}", strings.Repeat("x", 1<<20+1), `{"reserved_ip":{},"reserved_ip":{}}`} {
				r := p.Decode(c.status, []byte(b), nil, time.Now())
				if r.Outcome != "UNKNOWN" {
					t.Fatalf("malformed success: %+v", r)
				}
			}
			s := c.spec
			s.ServerID = "124"
			if s.Provider == "vultr" || s.Provider == "upcloud" {
				s.ServerID = resource
			}
			q, e := Prepare(s)
			if e != nil {
				t.Fatal(e)
			}
			if p.Digest() == q.Digest() {
				t.Fatal("digest omitted intended server")
			}
		})
	}
}
func TestNoImplicitDefaultsOrUnsupportedMutations(t *testing.T) {
	for _, c := range cases() {
		for _, action := range []string{"", "ATTACH", "DETACH", "RELEASE", "CREATE_SERVER"} {
			s := c.spec
			s.Action = action
			if _, e := Prepare(s); e == nil {
				t.Fatal("unsupported action")
			}
		}
		for _, region := range []string{"", "../servers", "x?token=secret", "https://evil.invalid"} {
			s := c.spec
			s.Region = region
			if _, e := Prepare(s); e == nil {
				t.Fatal("bad region")
			}
		}
		s := c.spec
		s.ServerID = ""
		if _, e := Prepare(s); e == nil {
			t.Fatal("unassigned target")
		}
	}
	for _, s := range []Spec{
		{Provider: "linode", Family: 4, Product: "public", Action: "ALLOCATE", Region: "us-east", ServerID: "123"},
		{Provider: "linode", Family: 6, Product: "routed", Action: "ALLOCATE", Region: "us-east", ServerID: "123", InterfaceMode: "legacy"},
		{Provider: "upcloud", Family: 4, Product: "public", Action: "ALLOCATE", Region: "fi-hel2", ServerID: server},
		{Provider: "upcloud", Family: 6, Product: "floating", Action: "ALLOCATE", Region: "fi-hel2", ServerID: server},
		{Provider: "upcloud", Family: 6, Product: "public", Action: "ALLOCATE", Region: "fi-hel2", ServerID: server, ReleasePolicy: "keep"},
	} {
		if _, e := Prepare(s); e == nil {
			t.Fatalf("implicit or unsupported spec: %+v", s)
		}
	}
}
func TestWrongScopeAndTypeRemainUnknown(t *testing.T) {
	for _, c := range cases() {
		p, _ := Prepare(c.spec)
		invalid := []string{strings.Replace(c.reply, c.spec.Region, "wrong-region", 1)}
		if c.spec.Provider == "linode" && c.spec.Family == 6 {
			invalid = []string{strings.Replace(c.reply, "/56", "/64", 1), strings.Replace(c.reply, "2001:db8::1", "::ffff:192.0.2.1", 1)}
		}
		if c.name == "do4" {
			invalid = append(invalid, strings.Replace(c.reply, "null", `{"id":123}`, 1))
		}
		if c.name == "linode4" {
			invalid = append(invalid, strings.Replace(c.reply, `"reserved":false`, `"reserved":true`, 1), strings.Replace(c.reply, `"linode_id":123`, `"linode_id":124`, 1))
		}
		if c.name == "vultr6" {
			invalid = append(invalid, strings.Replace(c.reply, "2001:db8:2::", "2001:db8:2::1", 1))
		}
		if c.spec.Provider == "upcloud" {
			invalid = append(invalid, strings.Replace(c.reply, `"access":"public"`, `"access":"utility"`, 1), strings.Replace(c.reply, `"access":"public"`, `"is_delegated":"yes","access":"public"`, 1))
		}
		for _, b := range invalid {
			if b == c.reply {
				t.Fatal("invalid test fixture")
			}
			if r := p.Decode(c.status, []byte(b), nil, time.Now()); r.Outcome != "UNKNOWN" {
				t.Fatalf("%s accepted mismatch: %+v", c.name, r)
			}
		}
	}
}
func TestRateLimitNoRawErrorLeak(t *testing.T) {
	p, _ := Prepare(cases()[0].spec)
	now := time.Unix(10000, 0)
	body := []byte(`{"message":"secret bearer credential"}`)
	r := p.Decode(429, body, http.Header{"Retry-After": []string{"120"}}, now)
	if r.Outcome != "UNKNOWN" || r.ErrorClass != "RATE_LIMIT" || r.RetryAt == nil || !r.RetryAt.Equal(now.Add(120*time.Second)) {
		t.Fatalf("%+v", r)
	}
	b, _ := json.Marshal(r)
	if bytes.Contains(b, []byte("secret")) {
		t.Fatal("error body leaked")
	}
	for _, v := range []string{"-1", "overflow9999999999999999999", "bad"} {
		if p.Decode(429, body, http.Header{"Retry-After": []string{v}}, now).RetryAt != nil {
			t.Fatal("invalid retry-after")
		}
	}
}
func FuzzResponseNeverPanics(f *testing.F) {
	for _, c := range cases() {
		f.Add(c.reply, c.status)
	}
	f.Fuzz(func(t *testing.T, b string, status int) {
		for _, c := range cases() {
			p, _ := Prepare(c.spec)
			r := p.Decode(status, []byte(b), nil, time.Now())
			if r.Outcome != "UNKNOWN" && r.HTTPStatus != c.status {
				t.Fatal(fmt.Sprint(r))
			}
		}
	})
}

func TestAmbiguousJSONAndDepthRemainUnknown(t *testing.T) {
	c := cases()[0]
	p, _ := Prepare(c.spec)
	for _, body := range []string{
		"{\"reserved_ip\":{},\"RESERVED_IP\":{\"ip\":\"203.0.113.7\",\"region\":{\"slug\":\"nyc3\"}}}",
		strings.Repeat("[", 1000) + "0" + strings.Repeat("]", 1000),
	} {
		if got := p.Decode(c.status, []byte(body), nil, time.Now()); got.Outcome != "UNKNOWN" {
			t.Fatalf("%+v", got)
		}
	}
}

func TestUnknownRetainsUntrustedResourceCandidates(t *testing.T) {
	for _, c := range cases() {
		p, _ := Prepare(c.spec)
		// No raw identity is promoted to success on unexpected status.
		got := p.Decode(500, []byte(c.reply), nil, time.Now())
		if got.Outcome != "UNKNOWN" || got.ResourceID != "" || got.Contract != "" || len(got.Candidates) != 1 || got.Candidates[0].ResourceID != c.id {
			t.Fatalf("%s lost candidate: %+v", c.name, got)
		}
		if c.spec.Provider != "linode" || c.spec.Family != 6 {
			b := strings.Replace(c.reply, c.spec.Region, "wrong-region", 1)
			got = p.Decode(c.status, []byte(b), nil, time.Now())
			if got.Outcome != "UNKNOWN" || got.ResourceID != "" || len(got.Candidates) != 1 || got.Candidates[0].ResourceID != c.id {
				t.Fatalf("%s lost mismatched candidate: %+v", c.name, got)
			}
		}
	}
}
func TestUnicodeCaseCollisionsRemainUnknownWithoutCandidates(t *testing.T) {
	c := cases()[6]
	p, _ := Prepare(c.spec)
	for _, pair := range []string{
		`"server":"wrong-id","ſerver":"` + server + `"`,
		`"ſerver":"` + server + `","server":"wrong-id"`,
	} {
		b := strings.Replace(c.reply, `"server":"`+server+`"`, pair, 1)
		got := p.Decode(c.status, []byte(b), nil, time.Now())
		if got.Outcome != "UNKNOWN" || got.ResourceID != "" || len(got.Candidates) != 0 {
			t.Fatalf("ambiguous Unicode reply: %+v", got)
		}
	}
}

func TestCaseVariantUnknownRetainsCandidate(t *testing.T) {
	c := cases()[0]
	p, _ := Prepare(c.spec)
	got := p.Decode(503, []byte(strings.Replace(c.reply, "reserved_ip", "RESERVED_IP", 1)), nil, time.Now())
	if got.Outcome != "UNKNOWN" || len(got.Candidates) != 1 || got.Candidates[0].ResourceID != c.id {
		t.Fatalf("%+v", got)
	}
}

func TestMalformedMetadataRetainsIdentityWithoutSuccess(t *testing.T) {
	for _, c := range cases() {
		p, _ := Prepare(c.spec)
		b := c.reply
		switch c.spec.Provider {
		case "digitalocean":
			b = strings.Replace(b, `"droplet":null`, `"droplet":"malformed"`, 1)
		case "vultr":
			b = strings.Replace(b, `"subnet_size":`, `"subnet_size":"bad","ignored":`, 1)
		case "linode":
			if c.spec.Family == 4 {
				b = strings.Replace(b, `"linode_id":123`, `"linode_id":"bad"`, 1)
			} else {
				b = strings.Replace(b, `"route_target":"2001:db8::1"`, `"route_target":{}`, 1)
			}
		case "upcloud":
			b = strings.Replace(b, `"zone":"fi-hel2"`, `"zone":123`, 1)
		}
		got := p.Decode(c.status, []byte(b), nil, time.Now())
		if got.Outcome != "UNKNOWN" || got.ResourceID != "" || got.Contract != "" || len(got.Candidates) != 1 || got.Candidates[0].ResourceID != c.id {
			t.Fatalf("%s lost identity: %+v", c.name, got)
		}
	}
}
func TestUpCloudDelegationEnum(t *testing.T) {
	for _, c := range cases() {
		if c.spec.Provider != "upcloud" {
			continue
		}
		p, _ := Prepare(c.spec)
		for _, value := range []string{`"unexpected"`, `""`, "null", "false", `"yes"`} {
			b := strings.Replace(c.reply, `"access":"public"`, `"access":"public","is_delegated":`+value, 1)
			got := p.Decode(c.status, []byte(b), nil, time.Now())
			if got.Outcome != "UNKNOWN" || len(got.Candidates) != 1 || got.Candidates[0].ResourceID != c.id {
				t.Fatalf("%s invalid enum: %+v", c.name, got)
			}
		}
		b := strings.Replace(c.reply, `"access":"public"`, `"access":"public","is_delegated":"no"`, 1)
		if got := p.Decode(c.status, []byte(b), nil, time.Now()); got.Outcome != "ACQUIRED" {
			t.Fatalf("explicit no: %+v", got)
		}
	}
}

func TestOverflowMetadataRetainsIdentity(t *testing.T) {
	c := cases()[3]
	p, _ := Prepare(c.spec)
	for _, number := range []string{"1e400", "-1e400", "999999999999999999999999999999999999999"} {
		b := strings.Replace(c.reply, "\"subnet_size\":64", "\"subnet_size\":"+number, 1)
		got := p.Decode(c.status, []byte(b), nil, time.Now())
		if got.Outcome != "UNKNOWN" || got.ResourceID != "" || len(got.Candidates) != 1 || got.Candidates[0].ResourceID != c.id {
			t.Fatalf("overflow lost identity: %+v", got)
		}
	}
}
