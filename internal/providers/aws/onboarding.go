package aws

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
	"net/url"
	"sort"
	"strings"
	"time"
)

func (Factory) Discover(ctx context.Context, r providers.OpenRequest, selection map[string]string) (providers.Discovery, error) {
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	var cred credential
	if cloudutil.ReadCredential(ctx, r.Credentials, &cred) != nil {
		return providers.Discovery{}, problem(providers.ErrorAuthentication, "discovery", "INVALID_CREDENTIAL_JSON", 0)
	}
	if strings.HasPrefix(cred.AccessKey, "ASIA") && cred.SessionToken == "" {
		return providers.Discovery{}, problem(providers.ErrorInvalidRequest, "discovery", "TEMPORARY_SESSION_TOKEN_REQUIRED", 0)
	}
	if selection["region"] != "" {
		cred.Region = selection["region"]
	}
	if cred.Region == "" {
		cred.Region = "eu-west-1"
	}
	source := cloudutil.NewJSONCredential(cred)
	if _, e := signingCredentials(ctx, source); e != nil {
		return providers.Discovery{}, e
	}
	c := &client{http: cloudutil.HTTPClient(r.HTTPClient), credentials: source, region: cred.Region}
	var rr struct {
		Set *struct {
			Items []struct {
				Name   string `xml:"regionName"`
				Status string `xml:"optInStatus"`
			} `xml:"item"`
		} `xml:"regionInfo"`
	}
	if e := c.call(ctx, "ec2", "DescribeRegions", url.Values{"AllRegions": {"false"}}, false, &rr); e != nil {
		return providers.Discovery{}, e
	}
	if rr.Set == nil {
		return providers.Discovery{}, problem(providers.ErrorUnavailable, "discovery", "REGIONS_MISSING", 200)
	}
	opts := []providers.DiscoveryOption{}
	seen := map[string]bool{}
	for _, x := range rr.Set.Items {
		if !regionPattern.MatchString(x.Name) || seen[x.Name] {
			return providers.Discovery{}, problem(providers.ErrorUnavailable, "discovery", "INVALID_REGION_LIST", 200)
		}
		seen[x.Name] = true
		if x.Status == "opt-in-not-required" || x.Status == "opted-in" {
			opts = append(opts, providers.DiscoveryOption{Value: x.Name, Label: x.Name})
		}
	}
	sort.Slice(opts, func(i, j int) bool { return opts[i].Value < opts[j].Value })
	valid := false
	for _, o := range opts {
		if o.Value == cred.Region {
			valid = true
		}
	}
	if !valid {
		return providers.Discovery{}, problem(providers.ErrorInvalidRequest, "discovery", "REGION_NOT_ENABLED", 0)
	}
	result := providers.Discovery{Fields: []providers.DiscoveryField{{Key: "region", Label: "AWS account region", Value: cred.Region, Options: opts}}, Resolved: map[string]any{"region": cred.Region}}
	d := &Driver{client: c, accountID: r.AccountID}
	if _, e := d.Account(ctx); e != nil {
		return providers.Discovery{}, e
	}
	subnets, e := d.publicSubnets(ctx, "")
	if e != nil {
		return providers.Discovery{}, e
	}
	if len(subnets) == 0 {
		result.Message = "No supported public subnet in this region. Choose another enabled region, or configure a public subnet with an attached Internet Gateway and unrestricted IPv4 network ACL. No network was changed."
		return result, nil
	}
	sort.Slice(subnets, func(i, j int) bool {
		if subnets[i].Default != subnets[j].Default {
			return subnets[i].Default
		}
		return subnets[i].ID < subnets[j].ID
	})
	subnetOpts := []providers.DiscoveryOption{}
	for _, s := range subnets {
		subnetOpts = append(subnetOpts, providers.DiscoveryOption{Value: s.ID, Label: s.ID + " / " + s.AZ + " / " + s.VPC})
	}
	if selection["subnet_id"] != "" {
		cred.Subnet = selection["subnet_id"]
	}
	if cred.Subnet == "" {
		cred.Subnet = subnets[0].ID
	}
	valid = false
	for _, s := range subnets {
		if s.ID == cred.Subnet {
			valid = true
		}
	}
	if !valid {
		return providers.Discovery{}, problem(providers.ErrorInvalidRequest, "discovery", "SUBNET_NOT_PUBLIC_OR_ACCESSIBLE", 0)
	}
	result.Fields = append(result.Fields, providers.DiscoveryField{Key: "subnet_id", Label: "Verified public subnet", Value: cred.Subnet, Options: subnetOpts})
	if len(cred.Groups) == 0 {
		cred.ManagedNetwork = true
		result.Message = "Automatic network: a dedicated security group will be created before the first server. It allows inbound IPv4 TCP/UDP ports 1–65535 for SSH and dynamic service ports; AWS default outbound rules apply. Only managed servers use it. Existing groups are unchanged. No server or network resource has been created by this preview."
	} else {
		cred.ManagedNetwork = false
		if e := d.validateNetwork(ctx, cred); e != nil {
			return providers.Discovery{}, e
		}
		result.Message = "Existing security groups selected. Their ingress rules remain your responsibility. No network resources will be created."
	}
	result.Complete = true
	result.Resolved["subnet_id"] = cred.Subnet
	result.Resolved["managed_network"] = cred.ManagedNetwork
	result.Credentials = cloudutil.NewJSONCredential(cred)
	return result, nil
}
