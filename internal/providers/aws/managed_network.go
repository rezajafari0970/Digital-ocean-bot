package aws

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

type groupPermission struct {
	Protocol string `xml:"ipProtocol"`
	From     int    `xml:"fromPort"`
	To       int    `xml:"toPort"`
	Ranges   []struct {
		CIDR string `xml:"cidrIp"`
	} `xml:"ipRanges>item"`
	IPv6 []struct {
		CIDR string `xml:"cidrIpv6"`
	} `xml:"ipv6Ranges>item"`
	Groups []struct {
		ID string `xml:"groupId"`
	} `xml:"groups>item"`
	Prefixes []struct {
		ID string `xml:"prefixListId"`
	} `xml:"prefixListIds>item"`
}
type securityGroup struct {
	ID      string `xml:"groupId"`
	Name    string `xml:"groupName"`
	VPC     string `xml:"vpcId"`
	Tags    []tag  `xml:"tagSet>item"`
	Ingress *struct {
		Items []groupPermission `xml:"item"`
	} `xml:"ipPermissions"`
}

func (d *Driver) networkName(vpc string) string {
	return "dob-network-" + hash("aws-network-v1\x00" + d.accountID + "\x00" + d.client.region + "\x00" + vpc)[:40]
}
func (d *Driver) networkTags(record providers.AccountResource) []tag {
	return []tag{{"dob:managed", "true"}, {"dob:account", d.accountID}, {"dob:purpose", "account-network"}, {"dob:intent", record.Nonce}}
}
func (d *Driver) findNetwork(ctx context.Context, record providers.AccountResource, vpc string) (*securityGroup, error) {
	groups, e := pages(ctx, d.client, "DescribeSecurityGroups", "securityGroupInfo", url.Values{"Filter.1.Name": {"vpc-id"}, "Filter.1.Value.1": {vpc}, "Filter.2.Name": {"group-name"}, "Filter.2.Value.1": {record.Name}}, func(g securityGroup) string { return g.ID })
	if e != nil {
		return nil, e
	}
	if len(groups) == 0 {
		return nil, nil
	}
	if len(groups) != 1 {
		return nil, problem(providers.ErrorUnavailable, "network", "AMBIGUOUS_SECURITY_GROUP", 200)
	}
	g := groups[0]
	if !idPattern.MatchString(g.ID) || !strings.HasPrefix(g.ID, "sg-") || g.VPC != vpc || g.Name != record.Name || g.Ingress == nil || (record.ID != "" && record.ID != g.ID) {
		return nil, problem(providers.ErrorPermissionDenied, "network", "SECURITY_GROUP_OWNERSHIP_MISMATCH", 0)
	}
	seen := map[string]bool{}
	for _, t := range g.Tags {
		if seen[t.Key] {
			return nil, problem(providers.ErrorUnavailable, "network", "DUPLICATE_SECURITY_GROUP_TAG", 200)
		}
		seen[t.Key] = true
	}
	for _, t := range d.networkTags(record) {
		found := false
		for _, actual := range g.Tags {
			if t == actual {
				found = true
			}
		}
		if !found {
			return nil, problem(providers.ErrorPermissionDenied, "network", "SECURITY_GROUP_OWNERSHIP_MISMATCH", 0)
		}
	}
	return &g, nil
}
func networkRules(g securityGroup) (map[string]bool, error) {
	seen := map[string]bool{}
	if g.Ingress == nil {
		return nil, problem(providers.ErrorUnavailable, "network", "SECURITY_GROUP_RULES_MISSING", 200)
	}
	for _, p := range g.Ingress.Items {
		protocol := p.Protocol
		if protocol == "6" {
			protocol = "tcp"
		}
		if protocol == "17" {
			protocol = "udp"
		}
		if (protocol != "tcp" && protocol != "udp") || seen[protocol] || p.From != 1 || p.To != 65535 || len(p.Ranges) != 1 || p.Ranges[0].CIDR != "0.0.0.0/0" || len(p.IPv6)+len(p.Groups)+len(p.Prefixes) > 0 {
			return nil, problem(providers.ErrorPermissionDenied, "network", "MANAGED_SECURITY_GROUP_RULES_CHANGED", 0)
		}
		seen[protocol] = true
	}
	return seen, nil
}
func (d *Driver) ensureNetwork(ctx context.Context, cred credential) (string, error) {
	if d.resources == nil {
		return "", problem(providers.ErrorUnavailable, "network", "RESOURCE_JOURNAL_UNAVAILABLE", 0)
	}
	subnets, e := d.publicSubnets(ctx, cred.Subnet)
	if e != nil {
		return "", e
	}
	if len(subnets) != 1 {
		return "", problem(providers.ErrorInvalidRequest, "network", "PUBLIC_SUBNET_UNAVAILABLE", 0)
	}
	vpc := subnets[0].VPC
	scope := d.client.region + ":" + vpc
	name := d.networkName(vpc)
	result := ""
	e = d.resources.WithLock(ctx, scope, name, func(session providers.AccountResourceSession) error {
		record := session.Record()
		if record.Complete {
			return problem(providers.ErrorPermissionDenied, "network", "NETWORK_ALREADY_RETIRED", 0)
		}
		group, e := d.findNetwork(ctx, record, vpc)
		if e != nil {
			return e
		}
		if group == nil {
			if record.ID != "" {
				return problem(providers.ErrorUnavailable, "network", "MANAGED_SECURITY_GROUP_MISSING", 0)
			}
			allowed, e := session.BeginAttempt(ctx)
			if e != nil {
				return e
			}
			if !allowed {
				return problem(providers.ErrorUnavailable, "network", "NETWORK_CREATE_AWAITING_OBSERVATION", 0)
			}
			q := url.Values{"VpcId": {vpc}, "GroupName": {name}, "GroupDescription": {"Digital Ocean Bot account network; managed lifecycle"}, "TagSpecification.1.ResourceType": {"security-group"}}
			for i, t := range d.networkTags(record) {
				prefix := fmt.Sprintf("TagSpecification.1.Tag.%d.", i+1)
				q.Set(prefix+"Key", t.Key)
				q.Set(prefix+"Value", t.Value)
			}
			var created struct {
				ID string `xml:"groupId"`
			}
			if e = d.client.call(ctx, "ec2", "CreateSecurityGroup", q, true, &created); e != nil {
				return e
			}
			if !idPattern.MatchString(created.ID) || !strings.HasPrefix(created.ID, "sg-") {
				return problem(providers.ErrorUnavailable, "network", "NETWORK_CREATE_UNCONFIRMED", 200)
			}
			if e = session.Bind(ctx, created.ID); e != nil {
				return e
			}
			record = session.Record()
			group, e = d.findNetwork(ctx, record, vpc)
			if e != nil {
				return e
			}
			if group == nil {
				return problem(providers.ErrorUnavailable, "network", "NETWORK_CREATE_AWAITING_OBSERVATION", 0)
			}
		}
		if e = session.Bind(ctx, group.ID); e != nil {
			return e
		}
		rules, e := networkRules(*group)
		if e != nil {
			return e
		}
		q := url.Values{"GroupId": {group.ID}}
		n := 0
		for _, protocol := range []string{"tcp", "udp"} {
			if rules[protocol] {
				continue
			}
			n++
			prefix := fmt.Sprintf("IpPermissions.%d.", n)
			q.Set(prefix+"IpProtocol", protocol)
			q.Set(prefix+"FromPort", "1")
			q.Set(prefix+"ToPort", "65535")
			q.Set(prefix+"IpRanges.1.CidrIp", "0.0.0.0/0")
		}
		if n > 0 {
			var accepted struct {
				OK bool `xml:"return"`
			}
			if e = d.client.call(ctx, "ec2", "AuthorizeSecurityGroupIngress", q, true, &accepted); e != nil {
				return e
			}
			if !accepted.OK {
				return problem(providers.ErrorUnavailable, "network", "NETWORK_RULES_UNCONFIRMED", 200)
			}
			group, e = d.findNetwork(ctx, session.Record(), vpc)
			if e != nil {
				return e
			}
			if group == nil {
				return problem(providers.ErrorUnavailable, "network", "NETWORK_RULES_AWAITING_OBSERVATION", 0)
			}
			rules, e = networkRules(*group)
			if e != nil {
				return e
			}
		}
		if !rules["tcp"] || !rules["udp"] {
			return problem(providers.ErrorUnavailable, "network", "NETWORK_RULES_AWAITING_OBSERVATION", 0)
		}
		result = group.ID
		return nil
	})
	if e != nil {
		if providers.Class(e) == providers.ErrorUnknown {
			return "", problem(providers.ErrorUnavailable, "network", "NETWORK_JOURNAL_FAILED_BEFORE_COMPUTE", 0)
		}
		// No RunInstances request has been issued. Reconcile the network journal on
		// the next attempt rather than treating this as an unknown compute create.
		return "", e
	}
	return result, nil
}
func (d *Driver) CleanupAccountResources(ctx context.Context) error {
	if d.resources == nil {
		return problem(providers.ErrorUnavailable, "network_cleanup", "RESOURCE_JOURNAL_UNAVAILABLE", 0)
	}
	records, e := d.resources.List(ctx)
	if e != nil {
		return e
	}
	for _, record := range records {
		parts := strings.Split(record.Scope, ":")
		if len(parts) != 2 || parts[0] != d.client.region || !idPattern.MatchString(parts[1]) || !strings.HasPrefix(parts[1], "vpc-") || record.Name != d.networkName(parts[1]) {
			return problem(providers.ErrorPermissionDenied, "network_cleanup", "NETWORK_SCOPE_MISMATCH", 0)
		}
		vpc := parts[1]
		e = d.resources.WithLock(ctx, record.Scope, record.Name, func(session providers.AccountResourceSession) error {
			current := session.Record()
			if current.Complete {
				return nil
			}
			g, e := d.findNetwork(ctx, current, vpc)
			if e != nil {
				return e
			}
			if g == nil {
				stable, e := session.Missing(ctx)
				if e != nil {
					return e
				}
				if !stable {
					return problem(providers.ErrorUnavailable, "network_cleanup", "NETWORK_ABSENCE_UNCONFIRMED", 0)
				}
				return session.Finish(ctx)
			}
			if e = session.Bind(ctx, g.ID); e != nil {
				return e
			}
			enis, e := pages(ctx, d.client, "DescribeNetworkInterfaces", "networkInterfaceSet", url.Values{"Filter.1.Name": {"group-id"}, "Filter.1.Value.1": {g.ID}}, func(x struct {
				ID string `xml:"networkInterfaceId"`
			}) string {
				return x.ID
			})
			if e != nil {
				return e
			}
			if len(enis) > 0 {
				return problem(providers.ErrorUnavailable, "network_cleanup", "NETWORK_STILL_ATTACHED", 0)
			}
			var accepted struct {
				OK bool `xml:"return"`
			}
			if e = d.client.call(ctx, "ec2", "DeleteSecurityGroup", url.Values{"GroupId": {g.ID}}, true, &accepted); e != nil {
				return e
			}
			// Even a successful deletion waits for stable fresh absence, across restarts.
			return problem(providers.ErrorUnavailable, "network_cleanup", "NETWORK_ABSENCE_UNCONFIRMED", 0)
		})
		if e != nil {
			return e
		}
	}
	return nil
}

var _ providers.AccountResourceCleaner = (*Driver)(nil)
