package aws

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"
)

func discoverRegion(ctx context.Context, r providers.OpenRequest, selection map[string]string) (providers.Discovery, error) {
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	var cred credential
	if cloudutil.ReadCredential(ctx, r.Credentials, &cred) != nil {
		return providers.Discovery{}, problem(providers.ErrorAuthentication, "discovery", "INVALID_CREDENTIAL_JSON", 0)
	}
	cred.Networks = nil
	if selection["region"] != "" {
		cred.Region = selection["region"]
	}
	if cred.Region == "" {
		cred.Region = "eu-west-1"
	}
	source := cloudutil.NewJSONCredential(cred)
	normalized, e := signingCredentials(ctx, source)
	if e != nil {
		return providers.Discovery{}, e
	}
	cred = normalized
	source = cloudutil.NewJSONCredential(cred)
	c := &client{http: cloudutil.HTTPClient(r.HTTPClient), credentials: source, region: cred.Region}
	// Prove the key's identity before interpreting regional EC2 access failures.
	d := &Driver{client: c, accountID: r.AccountID}
	if _, e := d.Account(ctx); e != nil {
		return providers.Discovery{}, e
	}
	var rr struct {
		Set *struct {
			Items []struct {
				Name   string `xml:"regionName"`
				Status string `xml:"optInStatus"`
			} `xml:"item"`
		} `xml:"regionInfo"`
	}
	if e := c.call(ctx, "ec2", "DescribeRegions", url.Values{"AllRegions": {"false"}}, false, &rr); e != nil {
		if Diagnostic(e).Code == "AuthFailure" {
			return providers.Discovery{}, &identityVerifiedEC2Error{cause: e}
		}
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
	result := providers.Discovery{Fields: []providers.DiscoveryField{{Key: "region", Label: "Primary AWS region", Value: cred.Region, Options: opts}}, Resolved: map[string]any{"region": cred.Region}}
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
	result.NetworkApproval = cred.ManagedNetwork
	result.Resolved["subnet_id"] = cred.Subnet
	result.Resolved["managed_network"] = cred.ManagedNetwork
	result.Credentials = cloudutil.NewJSONCredential(cred)
	return result, nil
}

// Additional regions are explicit and bounded. Every selected region is
// independently checked before its subnet is pinned into the credential.
func (Factory) Discover(ctx context.Context, r providers.OpenRequest, selection map[string]string) (providers.Discovery, error) {
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	result, e := discoverRegion(ctx, r, selection)
	if e != nil || len(result.Fields) == 0 {
		return result, e
	}
	options := append([]providers.DiscoveryOption{{Value: "", Label: "None"}}, result.Fields[0].Options...)
	for i := 2; i <= 5; i++ {
		key := "region_" + strconv.Itoa(i)
		result.Fields = append(result.Fields, providers.DiscoveryField{Key: key, Label: "Additional AWS region #" + strconv.Itoa(i-1) + " (optional)", Value: selection[key], Options: options})
	}
	if !result.Complete {
		return result, nil
	}
	cred, e := credentials(ctx, result.Credentials)
	if e != nil {
		return providers.Discovery{}, e
	}
	cred.Networks = map[string]regionalNetwork{}

	seen := map[string]bool{cred.Region: true}
	enabled := map[string]bool{}
	for _, option := range result.Fields[0].Options {
		enabled[option.Value] = true
	}
	type regionalDiscovery struct {
		region, subnetKey string
		result            providers.Discovery
	}
	extras := []regionalDiscovery{}
	for i := 2; i <= 5; i++ {
		region := selection["region_"+strconv.Itoa(i)]
		if region == "" {
			continue
		}
		if seen[region] {
			return providers.Discovery{}, problem(providers.ErrorInvalidRequest, "discovery", "DUPLICATE_REGION_SELECTION", 0)
		}
		if !enabled[region] {
			return providers.Discovery{}, problem(providers.ErrorInvalidRequest, "discovery", "REGION_NOT_ENABLED", 0)
		}
		seen[region] = true
		extras = append(extras, regionalDiscovery{region: region, subnetKey: "subnet_" + strconv.Itoa(i)})
	}
	// Three independent network reads at a time; no partial credential can escape.
	// Results merge in selection order after all goroutines have stopped.
	work, stop := context.WithCancel(ctx)
	defer stop()
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	var first sync.Once
	var failure error
	for i := range extras {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-work.Done():
				return
			}
			item := &extras[i]
			regional := cred
			regional.Region, regional.Subnet, regional.Groups, regional.ManagedNetwork, regional.Networks = item.region, "", nil, true, nil
			request := r
			request.Credentials = cloudutil.NewJSONCredential(regional)
			var err error
			item.result, err = discoverRegion(work, request, map[string]string{"region": item.region, "subnet_id": selection[item.subnetKey]})
			if err != nil {
				first.Do(func() { failure = err; stop() })
			}
		}(i)
	}
	wg.Wait()
	if failure != nil {
		return providers.Discovery{}, failure
	}
	if err := ctx.Err(); err != nil {
		return providers.Discovery{}, err
	}
	for _, item := range extras {
		extra := item.result
		for _, field := range extra.Fields {
			if field.Key == "subnet_id" {
				field.Key = item.subnetKey
				field.Label = "Public subnet — " + item.region
				result.Fields = append(result.Fields, field)
			}
		}
		if !extra.Complete {
			result.Complete = false
			result.Credentials = nil
			result.Message = item.region + ": " + extra.Message
			return result, nil
		}
		network, err := credentials(ctx, extra.Credentials)
		if err != nil {
			return providers.Discovery{}, err
		}
		cred.Networks[item.region] = regionalNetwork{network.Subnet, network.Groups, network.ManagedNetwork}
		result.NetworkApproval = result.NetworkApproval || network.ManagedNetwork
	}
	result.Resolved["regional_networks"] = cred.Networks
	result.Credentials = cloudutil.NewJSONCredential(cred)
	result.Message += " Select up to four additional enabled regions above, then reload to verify their networks. Random region choices include only these verified regions. Each additional region uses its own dedicated automatic security group."
	return result, nil
}
