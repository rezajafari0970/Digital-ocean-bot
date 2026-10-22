package aws

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/url"
	"sort"
	"strings"
)

type subnetData struct {
	ID        string `xml:"subnetId"`
	VPC       string `xml:"vpcId"`
	State     string `xml:"state"`
	AZ        string `xml:"availabilityZone"`
	Default   bool   `xml:"defaultForAz"`
	Available int    `xml:"availableIpAddressCount"`
}
type routeTable struct {
	ID           string `xml:"routeTableId"`
	VPC          string `xml:"vpcId"`
	Associations []struct {
		Subnet string `xml:"subnetId"`
		Main   bool   `xml:"main"`
		State  struct {
			State string `xml:"state"`
		} `xml:"associationState"`
	} `xml:"associationSet>item"`
	Routes []struct {
		CIDR    string `xml:"destinationCidrBlock"`
		Gateway string `xml:"gatewayId"`
		State   string `xml:"state"`
	} `xml:"routeSet>item"`
}
type networkACL struct {
	ID           string `xml:"networkAclId"`
	VPC          string `xml:"vpcId"`
	Associations []struct {
		Subnet string `xml:"subnetId"`
	} `xml:"associationSet>item"`
	Entries []struct {
		Number   int    `xml:"ruleNumber"`
		Protocol string `xml:"protocol"`
		Action   string `xml:"ruleAction"`
		Egress   bool   `xml:"egress"`
		CIDR     string `xml:"cidrBlock"`
	} `xml:"entrySet>item"`
}
type gatewayData struct {
	ID          string `xml:"internetGatewayId"`
	Attachments []struct {
		VPC   string `xml:"vpcId"`
		State string `xml:"state"`
	} `xml:"attachmentSet>item"`
}

func aclOpen(a networkACL) bool {
	sort.Slice(a.Entries, func(i, j int) bool { return a.Entries[i].Number < a.Entries[j].Number })
	for _, out := range []bool{false, true} {
		ok := false
		seen := map[int]bool{}
		for _, e := range a.Entries {
			if e.Egress != out || e.CIDR == "" {
				continue
			}
			if e.Number <= 0 || seen[e.Number] {
				return false
			}
			seen[e.Number] = true
			if e.Action == "allow" && e.Protocol == "-1" && e.CIDR == "0.0.0.0/0" {
				ok = true
				break
			}
			if e.Action != "allow" {
				return false
			}
		}
		if !ok {
			return false
		}
	}
	return true
}
func effectiveGateway(s subnetData, tables []routeTable) string {
	var explicit, main []routeTable
	for _, t := range tables {
		if t.VPC != s.VPC {
			continue
		}
		for _, a := range t.Associations {
			if a.State.State != "" && a.State.State != "associated" {
				continue
			}
			if a.Subnet == s.ID {
				explicit = append(explicit, t)
			}
			if a.Main {
				main = append(main, t)
			}
		}
	}
	use := explicit
	if len(use) == 0 {
		use = main
	}
	if len(use) != 1 {
		return ""
	}
	found := ""
	for _, rt := range use[0].Routes {
		if rt.CIDR == "0.0.0.0/0" {
			if found != "" || rt.State != "active" || !strings.HasPrefix(rt.Gateway, "igw-") {
				return ""
			}
			found = rt.Gateway
		}
	}
	return found
}
func (d *Driver) publicSubnets(ctx context.Context, id string) ([]subnetData, error) {
	q := url.Values{}
	if id != "" {
		if !idPattern.MatchString(id) || !strings.HasPrefix(id, "subnet-") {
			return nil, problem(providers.ErrorInvalidRequest, "network", "INVALID_SUBNET_ID", 0)
		}
		q.Set("Filter.1.Name", "subnet-id")
		q.Set("Filter.1.Value.1", id)
	}
	subs, e := pages(ctx, d.client, "DescribeSubnets", "subnetSet", q, func(s subnetData) string { return s.ID })
	if e != nil {
		return nil, e
	}
	tables, e := pages(ctx, d.client, "DescribeRouteTables", "routeTableSet", nil, func(t routeTable) string { return t.ID })
	if e != nil {
		return nil, e
	}
	acls, e := pages(ctx, d.client, "DescribeNetworkAcls", "networkAclSet", nil, func(a networkACL) string { return a.ID })
	if e != nil {
		return nil, e
	}
	gateways, e := pages(ctx, d.client, "DescribeInternetGateways", "internetGatewaySet", nil, func(g gatewayData) string { return g.ID })
	if e != nil {
		return nil, e
	}
	out := []subnetData{}
	for _, s := range subs {
		if !idPattern.MatchString(s.ID) || !strings.HasPrefix(s.ID, "subnet-") || !idPattern.MatchString(s.VPC) || !strings.HasPrefix(s.VPC, "vpc-") {
			return nil, problem(providers.ErrorUnavailable, "network", "INVALID_SUBNET", 200)
		}
		if id != "" && s.ID != id {
			return nil, problem(providers.ErrorUnavailable, "network", "SUBNET_ID_MISMATCH", 200)
		}
		if s.State != "available" || s.Available < 1 {
			continue
		}
		gw := effectiveGateway(s, tables)
		if gw == "" {
			continue
		}
		attached := false
		for _, g := range gateways {
			if g.ID == gw {
				for _, a := range g.Attachments {
					if a.VPC == s.VPC && a.State == "available" {
						attached = true
					}
				}
			}
		}
		if !attached {
			continue
		}
		matches := 0
		open := false
		for _, a := range acls {
			if a.VPC != s.VPC {
				continue
			}
			for _, as := range a.Associations {
				if as.Subnet == s.ID {
					matches++
					open = aclOpen(a)
				}
			}
		}
		if matches != 1 || !open {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}
