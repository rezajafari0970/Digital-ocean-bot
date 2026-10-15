package ipwire

import (
	"encoding/json"
	"net/netip"
	"strconv"
	"strings"
)

func candidateAddress(raw string) (string, int) {
	a, e := netip.ParseAddr(raw)
	if e != nil || a.Zone() != "" || a.Is4In6() || !a.IsGlobalUnicast() {
		return "", 0
	}
	f := 6
	if a.Is4() {
		f = 4
	}
	return a.String(), f
}
func candidateRegion(raw string) string {
	if segment.MatchString(raw) {
		return raw
	}
	return ""
}
func candidateServer(raw string) string {
	if positiveID(raw) || uuid.MatchString(raw) {
		return raw
	}
	return ""
}
func object(raw []byte) map[string]json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	out := map[string]json.RawMessage{}
	for k, v := range m {
		out[strings.ToLower(k)] = v
	}
	return out
}
func field(m map[string]json.RawMessage, key string) string {
	var s string
	_ = json.Unmarshal(m[key], &s)
	return s
}
func integer(m map[string]json.RawMessage, key string) (int64, bool) {
	var n int64
	if len(m[key]) == 0 || string(m[key]) == "null" {
		return 0, false
	}
	e := json.Unmarshal(m[key], &n)
	return n, e == nil
}

// Identity fields decode independently of malformed metadata after strict JSON
// validation. Candidates remain untrusted and confer no cleanup authority.
func (p Prepared) candidates(b []byte) []Candidate {
	var validated json.RawMessage
	if strictJSON(b, &validated) != nil {
		return nil
	}
	root := object(b)
	out := []Candidate{}
	add := func(c Candidate) {
		if c.ResourceID != "" {
			out = append(out, c)
		}
	}
	switch p.spec.Provider {
	case "digitalocean":
		for _, key := range []string{"reserved_ip", "reserved_ipv6"} {
			x := object(root[key])
			a, f := candidateAddress(field(x, "ip"))
			c := Candidate{ResourceID: a, Address: a, Family: f}
			c.Region = candidateRegion(field(object(x["region"]), "slug"))
			if key == "reserved_ipv6" {
				c.Region = candidateRegion(field(x, "region_slug"))
			}
			if id, ok := integer(object(x["droplet"]), "id"); ok {
				c.ServerID = candidateServer(strconv.FormatInt(id, 10))
			}
			add(c)
		}
	case "vultr":
		x := object(root["reserved_ip"])
		id := field(x, "id")
		if !uuid.MatchString(id) {
			break
		}
		c := Candidate{ResourceID: id, Region: candidateRegion(field(x, "region")), ServerID: candidateServer(field(x, "instance_id"))}
		a, f := candidateAddress(field(x, "subnet"))
		c.Family = f
		if bits, ok := integer(x, "subnet_size"); a != "" && ok {
			if pre, e := netip.ParsePrefix(a + "/" + strconv.FormatInt(bits, 10)); e == nil && pre == pre.Masked() {
				c.Prefix = pre.String()
				if pre.Bits() == pre.Addr().BitLen() {
					c.Address = a
				}
			}
		}
		add(c)
	case "linode":
		if a, f := candidateAddress(field(root, "address")); a != "" {
			c := Candidate{ResourceID: a, Address: a, Family: f, Region: candidateRegion(field(root, "region"))}
			if id, ok := integer(root, "linode_id"); ok {
				c.ServerID = candidateServer(strconv.FormatInt(id, 10))
			}
			add(c)
		}
		if pre, e := netip.ParsePrefix(field(root, "range")); e == nil && pre == pre.Masked() && pre.Addr().Is6() && !pre.Addr().Is4In6() && pre.Addr().IsGlobalUnicast() && (pre.Bits() == 56 || pre.Bits() == 64) {
			add(Candidate{ResourceID: pre.String(), Prefix: pre.String(), Family: 6})
		}
	case "upcloud":
		x := object(root["ip_address"])
		a, f := candidateAddress(field(x, "address"))
		add(Candidate{ResourceID: a, Address: a, Family: f, Region: candidateRegion(field(x, "zone")), ServerID: candidateServer(field(x, "server"))})
	}
	return out
}
