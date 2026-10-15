package ipwire

import (
	"encoding/json"
	"fmt"
	"strconv"
)

func (p Prepared) created(b []byte) (Result, error) {
	s := p.spec
	out := Result{Region: s.Region}
	switch s.Provider {
	case "digitalocean":
		type data struct {
			IP         string
			Region     struct{ Slug string }
			RegionSlug string `json:"region_slug"`
			Droplet    json.RawMessage
		}
		var v struct {
			V4 *data `json:"reserved_ip"`
			V6 *data `json:"reserved_ipv6"`
		}
		if strictJSON(b, &v) != nil {
			return out, ErrSpec
		}
		x := v.V4
		if s.Family == 6 {
			x = v.V6
		}
		if x == nil || (len(x.Droplet) > 0 && string(x.Droplet) != "null") {
			return out, ErrSpec
		}
		region := x.Region.Slug
		if s.Family == 6 {
			region = x.RegionSlug
		}
		if region != s.Region {
			return out, ErrSpec
		}
		a, e := address(x.IP, s.Family)
		if e != nil {
			return out, e
		}
		out.ResourceID = a
		out.Address = a
	case "vultr":
		var v struct {
			IP *struct {
				ID, Region, Subnet string
				Type               string `json:"ip_type"`
				Bits               *int   `json:"subnet_size"`
				Server             string `json:"instance_id"`
			} `json:"reserved_ip"`
		}
		if strictJSON(b, &v) != nil || v.IP == nil {
			return out, ErrSpec
		}
		x := v.IP
		if !uuid.MatchString(x.ID) || x.Region != s.Region || x.Type != fmt.Sprintf("v%d", s.Family) || x.Server != "" || x.Bits == nil {
			return out, ErrSpec
		}
		bits := 32
		if s.Family == 6 {
			bits = s.PrefixLength
		}
		pre, e := prefix(x.Subnet+"/"+strconv.Itoa(*x.Bits), s.Family, bits)
		if e != nil {
			return out, e
		}
		out.ResourceID = x.ID
		out.Prefix = pre
		if s.Family == 4 {
			out.Address = x.Subnet
		}
	case "linode":
		if s.Family == 4 {
			var x struct {
				Address, Type, Region string
				Public                *bool
				Reserved              *bool
				LinodeID              int64 `json:"linode_id"`
			}
			if strictJSON(b, &x) != nil || x.Type != "ipv4" || x.Region != s.Region || x.Public == nil || !*x.Public || x.Reserved == nil || *x.Reserved || strconv.FormatInt(x.LinodeID, 10) != s.ServerID {
				return out, ErrSpec
			}
			a, e := address(x.Address, 4)
			if e != nil {
				return out, e
			}
			out.ResourceID = a
			out.Address = a
			out.ServerID = s.ServerID
		} else {
			var x struct {
				Range       string
				RouteTarget string `json:"route_target"`
			}
			if strictJSON(b, &x) != nil {
				return out, ErrSpec
			}
			pre, e := prefix(x.Range, 6, s.PrefixLength)
			if e != nil {
				return out, e
			}
			if _, e = address(x.RouteTarget, 6); e != nil {
				return out, e
			}
			out.ResourceID = pre
			out.Prefix = pre
			// This response does not prove which Linode owns the SLAAC target.
			// Fresh inventory must establish attachment before journal confirmation.
		}
	case "upcloud":
		var v struct {
			IP *struct {
				Address, Access, Family, Server, Zone, Floating string
				Delegated                                       json.RawMessage `json:"is_delegated"`
				ReleasePolicy                                   string          `json:"release_policy"`
			} `json:"ip_address"`
		}
		if strictJSON(b, &v) != nil || v.IP == nil {
			return out, ErrSpec
		}
		x := v.IP
		if x.Access != "public" || x.Family != fmt.Sprintf("IPv%d", s.Family) || x.Zone != s.Region || !nonDelegated(x.Delegated) {
			return out, ErrSpec
		}
		if s.Product == "floating" {
			if x.Floating != "yes" || x.Server != "" {
				return out, ErrSpec
			}
		} else if x.Floating != "no" || x.Server != s.ServerID {
			return out, ErrSpec
		}
		if s.Family == 4 && x.ReleasePolicy != s.ReleasePolicy {
			return out, ErrSpec
		}
		a, e := address(x.Address, s.Family)
		if e != nil {
			return out, e
		}
		out.ResourceID = a
		out.Address = a
		out.ServerID = x.Server
	default:
		return out, ErrUnsupported
	}
	return out, nil
}

// Omitted optional enum is distinct from null, empty or unknown values.
func nonDelegated(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var value string
	return json.Unmarshal(raw, &value) == nil && value == "no"
}
