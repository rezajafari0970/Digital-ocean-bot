package aws

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
)

type Factory struct{}

func (Factory) Name() string { return "aws" }
func (Factory) Metadata() providers.Metadata {
	return providers.Metadata{WebsiteURL: "https://console.aws.amazon.com/", Name: "aws", DisplayName: "Amazon AWS EC2", CredentialLabel: "AWS credentials", CredentialFields: []providers.CredentialField{
		{Key: "access_key_id", Label: "Access key ID", Secret: true}, {Key: "secret_access_key", Label: "Secret access key", Secret: true}, {Key: "session_token", Label: "Session token (temporary credentials)", Secret: true, Optional: true},
		{Key: "region", Label: "Region override", Optional: true, Advanced: true}, {Key: "subnet_id", Label: "Public subnet override", Optional: true, Advanced: true}, {Key: "security_group_ids", Label: "Existing security group IDs (comma-separated)", Optional: true, List: true, Advanced: true}},
		CredentialHelp: "Enter the access key pair. Enabled regions and a public subnet are discovered automatically. Review the selected network before saving. Temporary credentials also require their session token. No server is created during validation.", Status: "ready", Defaults: cloudutil.Defaults()}

}
func (Factory) Open(ctx context.Context, r providers.OpenRequest) (providers.Driver, error) {
	if strings.TrimSpace(r.AccountID) == "" || r.HTTPClient == nil || r.Credentials == nil {
		return nil, errors.New("AWS: invalid open request")
	}
	cred, e := credentials(ctx, r.Credentials)
	if e != nil {
		return nil, e
	}
	if len(cred.Networks) != 0 {
		return openRegional(r, cred)
	}
	return &Driver{client: &client{http: cloudutil.HTTPClient(r.HTTPClient), credentials: r.Credentials, region: cred.Region, expected: r.ExpectedExternalID}, accountID: r.AccountID, absence: r.Absence, resources: r.Resources}, nil
}

type Driver struct {
	resources providers.AccountResources
	client    *client
	accountID string
	absence   providers.AbsenceVerifier
}

func (d *Driver) Name() string { return "aws" }
func (d *Driver) Capabilities() providers.Capabilities {
	return providers.Capabilities{Account: true, Catalog: true, Compute: true, Inventory: true, InlineSSHKeys: true}
}
func (d *Driver) Health(ctx context.Context) error { _, e := d.Account(ctx); return e }
func (d *Driver) Account(ctx context.Context) (providers.Account, error) {
	var x struct {
		Result struct {
			Account string `xml:"Account"`
			ARN     string `xml:"Arn"`
		} `xml:"GetCallerIdentityResult"`
	}
	if e := d.client.call(ctx, "sts", "GetCallerIdentity", nil, false, &x); e != nil {
		return providers.Account{}, e
	}
	if !accountPattern.MatchString(x.Result.Account) || !strings.HasPrefix(x.Result.ARN, "arn:aws:") {
		return providers.Account{}, problem(providers.ErrorUnavailable, "account", "INVALID_ACCOUNT_IDENTITY", 200)
	}
	if d.client.expected != "" && x.Result.Account+":"+d.client.region != d.client.expected {
		return providers.Account{}, problem(providers.ErrorAuthentication, "account", "ACCOUNT_SCOPE_MISMATCH", 0)
	}
	return providers.Account{ID: x.Result.Account + ":" + d.client.region, Status: "active"}, nil
}

type tag struct {
	Key   string `xml:"key"`
	Value string `xml:"value"`
}
type instance struct {
	ID      string `xml:"instanceId"`
	Type    string `xml:"instanceType"`
	Image   string `xml:"imageId"`
	IP      string `xml:"ipAddress"`
	Created string `xml:"launchTime"`
	Token   string `xml:"clientToken"`
	State   struct {
		Name string `xml:"name"`
	} `xml:"instanceState"`
	Tags []tag `xml:"tagSet>item"`
}
type reservation struct {
	ID        string     `xml:"reservationId"`
	Instances []instance `xml:"instancesSet>item"`
}

func (d *Driver) ref(id string) string { return d.client.region + ":" + id }
func (d *Driver) native(ref string) (string, error) {
	p := strings.Split(ref, ":")
	if len(p) != 2 || p[0] != d.client.region || !idPattern.MatchString(p[1]) || !strings.HasPrefix(p[1], "i-") {
		return "", problem(providers.ErrorInvalidRequest, "server", "INVALID_REGIONAL_SERVER_ID", 0)
	}
	return p[1], nil
}
func tagValue(x instance, k string) string {
	for _, t := range x.Tags {
		if t.Key == k {
			return t.Value
		}
	}
	return ""
}
func (d *Driver) owns(x instance) bool {
	return tagValue(x, "dob:account") == d.accountID && tagValue(x, "dob:managed") == "true" && tagValue(x, "dob:deployment") != ""
}
func (d *Driver) normalize(x instance) providers.Server {
	state := providers.ServerStateUnknown
	ip := cloudutil.PublicIPv4(x.IP)
	switch x.State.Name {
	case "pending", "rebooting":
		state = providers.ServerStateProvisioning
	case "running":
		state = providers.ServerStateProvisioning
		if ip != "" {
			state = providers.ServerStateReady
		}
	case "stopping":
		state = providers.ServerStateStopping
	case "stopped":
		state = providers.ServerStateStopped
	case "shutting-down", "terminated":
		state = providers.ServerStateDeleting
	}
	created, _ := time.Parse(time.RFC3339, x.Created)
	tags := []string{}
	if d.owns(x) {
		tags = []string{"managed-by-digital-ocean-bot", "dob-account-" + d.accountID, tagValue(x, "dob:deployment")}
	}
	return providers.Server{ID: d.ref(x.ID), Name: tagValue(x, "Name"), State: state, Ready: state == providers.ServerStateReady, RegionID: d.client.region, PrimaryIPv4: ip, CreatedAt: created, Tags: tags, Metadata: map[string]any{"plan": x.Type, "image": x.Image, "provider_state": x.State.Name}}
}
func (d *Driver) instances(ctx context.Context, q url.Values) ([]instance, error) {
	rs, e := pages(ctx, d.client, "DescribeInstances", "reservationSet", q, func(x reservation) string { return x.ID })
	if e != nil {
		return nil, e
	}
	out := []instance{}
	seen := map[string]bool{}
	for _, r := range rs {
		if len(r.Instances) == 0 {
			return nil, problem(providers.ErrorUnavailable, "inventory", "EMPTY_RESERVATION", 200)
		}
		for _, x := range r.Instances {
			if !idPattern.MatchString(x.ID) || !strings.HasPrefix(x.ID, "i-") || x.State.Name == "" || seen[x.ID] {
				return nil, problem(providers.ErrorUnavailable, "inventory", "INVALID_INSTANCE", 200)
			}
			seen[x.ID] = true
			out = append(out, x)
		}
	}
	return out, nil
}
func (d *Driver) raw(ctx context.Context, id string) (instance, error) {
	native, e := d.native(id)
	if e != nil {
		return instance{}, e
	}
	// Filter avoids EC2's prohibition on MaxResults combined with InstanceId.N.
	xs, e := d.instances(ctx, url.Values{"Filter.1.Name": {"instance-id"}, "Filter.1.Value.1": {native}})
	if e != nil {
		if providers.IsClass(e, providers.ErrorNotFound) {
			if pending := providers.ConfirmAbsence(ctx, d.absence, id); pending != nil {
				return instance{}, pending
			}
		}
		return instance{}, e
	}
	if len(xs) == 0 {
		if e := providers.ConfirmAbsence(ctx, d.absence, id); e != nil {
			return instance{}, e
		}
		return instance{}, problem(providers.ErrorNotFound, "get_server", "INSTANCE_NOT_FOUND", 404)
	}
	if len(xs) != 1 || xs[0].ID != native {
		return instance{}, problem(providers.ErrorUnavailable, "get_server", "INSTANCE_ID_MISMATCH", 200)
	}
	if d.absence != nil {
		if e := d.absence.Present(ctx, id); e != nil {
			return instance{}, e
		}
	}
	return xs[0], nil
}
func (d *Driver) GetServer(ctx context.Context, id string) (providers.Server, error) {
	x, e := d.raw(ctx, id)
	if e != nil {
		return providers.Server{}, e
	}
	if x.State.Name == "terminated" {
		return providers.Server{}, problem(providers.ErrorNotFound, "get_server", "INSTANCE_TERMINATED", 404)
	}
	return d.normalize(x), nil
}
func (d *Driver) ListServers(ctx context.Context) ([]providers.Server, error) {
	xs, e := d.instances(ctx, nil)
	if e != nil {
		return nil, e
	}
	out := []providers.Server{}
	for _, x := range xs {
		if x.State.Name != "terminated" {
			if d.absence != nil {
				if e := d.absence.Present(ctx, d.ref(x.ID)); e != nil {
					return nil, e
				}
			}
			out = append(out, d.normalize(x))
		}
	}
	return out, nil
}
func (d *Driver) Inventory(ctx context.Context) (providers.Inventory, error) {
	ss, e := d.ListServers(ctx)
	if e != nil {
		return providers.Inventory{}, e
	}
	return providers.Inventory{Servers: ss, Resources: []providers.Resource{}, ObservedAt: time.Now().UTC()}, nil
}
func capacity(n int) providers.Capacity {
	return providers.Capacity{Source: "aws_regional_inventory_quota_unknown", ComputeInUse: n, LimitKnown: false, ObservedAt: time.Now().UTC()}
}
func (d *Driver) Capacity(ctx context.Context) (providers.Capacity, error) {
	inv, e := d.Inventory(ctx)
	if e != nil {
		return providers.Capacity{}, e
	}
	return capacity(len(inv.Servers)), nil
}
func (d *Driver) observe(ctx context.Context, full bool) (providers.Observation, error) {
	a, e := d.Account(ctx)
	if e != nil {
		return providers.Observation{}, e
	}
	inv, e := d.Inventory(ctx)
	if e != nil {
		return providers.Observation{}, e
	}
	cat := providers.Catalog{}
	if full {
		cat, e = d.Catalog(ctx)
		if e != nil {
			return providers.Observation{}, e
		}
	}
	return providers.Observation{Account: a, Inventory: inv, Capacity: capacity(len(inv.Servers)), Catalog: cat, ObservedAt: time.Now().UTC()}, nil
}
func (d *Driver) Observe(ctx context.Context) (providers.Observation, error) {
	return d.observe(ctx, true)
}
func (d *Driver) ObserveFast(ctx context.Context) (providers.Observation, error) {
	return d.observe(ctx, false)
}
func (d *Driver) FindServerByIdentity(ctx context.Context, identity string) ([]providers.Server, error) {
	if identity == "" {
		return nil, problem(providers.ErrorInvalidRequest, "find_server", "MISSING_IDENTITY", 0)
	}
	xs, e := d.instances(ctx, url.Values{"Filter.1.Name": {"client-token"}, "Filter.1.Value.1": {d.clientToken(identity)}})
	if e != nil {
		return nil, e
	}
	out := []providers.Server{}
	for _, x := range xs {
		if d.owns(x) && tagValue(x, "dob:deployment") == identity && x.State.Name != "terminated" {
			out = append(out, d.normalize(x))
		}
	}
	return out, nil
}
func (d *Driver) clientToken(identity string) string {
	return hash("aws-ec2-v1\x00" + d.accountID + "\x00" + identity)
}
func (d *Driver) DeleteServer(ctx context.Context, id string) error {
	x, e := d.raw(ctx, id)
	if providers.IsClass(e, providers.ErrorNotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	if x.State.Name == "terminated" {
		return nil
	}
	if !d.owns(x) {
		return problem(providers.ErrorPermissionDenied, "delete_server", "OWNERSHIP_NOT_PROVEN", 0)
	}
	var out struct {
		Items []struct {
			ID string `xml:"instanceId"`
		} `xml:"instancesSet>item"`
	}
	e = d.client.call(ctx, "ec2", "TerminateInstances", url.Values{"InstanceId.1": {x.ID}}, true, &out)
	if providers.IsClass(e, providers.ErrorNotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	if len(out.Items) != 1 || out.Items[0].ID != x.ID {
		return problem(providers.ErrorAmbiguousOutcome, "delete_server", "DELETE_UNCONFIRMED", 200)
	}
	return nil
}
func (d *Driver) validateNetwork(ctx context.Context, cred credential) error {
	if cred.ManagedNetwork {
		xs, e := d.publicSubnets(ctx, cred.Subnet)
		if e != nil {
			return e
		}
		if len(xs) != 1 {
			return problem(providers.ErrorInvalidRequest, "network", "PUBLIC_SUBNET_UNAVAILABLE", 0)
		}
		return nil
	}
	var subnet struct {
		Items []struct {
			ID    string `xml:"subnetId"`
			VPC   string `xml:"vpcId"`
			State string `xml:"state"`
		} `xml:"subnetSet>item"`
	}
	if e := d.client.call(ctx, "ec2", "DescribeSubnets", url.Values{"SubnetId.1": {cred.Subnet}}, false, &subnet); e != nil {
		return e
	}
	if len(subnet.Items) != 1 || subnet.Items[0].ID != cred.Subnet || subnet.Items[0].State != "available" || subnet.Items[0].VPC == "" {
		return problem(providers.ErrorInvalidRequest, "network", "SUBNET_UNAVAILABLE", 0)
	}
	q := url.Values{}
	for i, g := range cred.Groups {
		q.Set(fmt.Sprintf("GroupId.%d", i+1), g)
	}
	var groups struct {
		Items []struct {
			ID  string `xml:"groupId"`
			VPC string `xml:"vpcId"`
		} `xml:"securityGroupInfo>item"`
	}
	if e := d.client.call(ctx, "ec2", "DescribeSecurityGroups", q, false, &groups); e != nil {
		return e
	}
	seen := map[string]bool{}
	for _, g := range groups.Items {
		if !cloudutil.Contains(cred.Groups, g.ID) || seen[g.ID] || g.VPC != subnet.Items[0].VPC {
			return problem(providers.ErrorInvalidRequest, "network", "SECURITY_GROUP_VPC_MISMATCH", 0)
		}
		seen[g.ID] = true
	}
	if len(seen) != len(cred.Groups) {
		return problem(providers.ErrorUnavailable, "network", "INCOMPLETE_SECURITY_GROUPS", 200)
	}
	return nil
}
func (d *Driver) CreateServer(ctx context.Context, r providers.CreateServerRequest) (providers.CreateServerResult, error) {
	rejected := providers.CreateServerResult{Outcome: providers.OutcomeRejected, ComputeNotDispatched: true}
	invalid := func(code string) (providers.CreateServerResult, error) {
		return rejected, problem(providers.ErrorInvalidRequest, "create_server", code, 0)
	}
	if r.RegionID != d.client.region || r.Identity == "" || len(r.Identity) > 256 || strings.ContainsAny(r.Identity+r.Name, "\r\n\x00") || r.Name == "" || len(r.Name) > 255 || len(r.SSHKeyRefs) > 0 {
		return invalid("INVALID_CREATE_REQUEST")
	}
	data, e := cloudutil.CloudInit(r.SSHAuthorizedKeys)
	if e != nil {
		return invalid("INVALID_SSH_KEYS")
	}
	cred, e := credentials(ctx, d.client.credentials)
	if e != nil {
		return rejected, e
	}
	if e = d.validateNetwork(ctx, cred); e != nil {
		return rejected, e
	}
	images, e := d.images(ctx, url.Values{"ImageId.1": {r.ImageID}})
	if e != nil {
		return rejected, e
	}
	if len(images) != 1 || images[0].ID != r.ImageID || !usableImage(images[0]) {
		return invalid("UNSUPPORTED_IMAGE")
	}
	img := images[0]
	ps, e := d.plans(ctx, r.PlanID, img)
	if e != nil {
		return rejected, e
	}
	if len(ps) != 1 || ps[0].ID != r.PlanID {
		return invalid("UNSUPPORTED_PLAN")
	}
	if cred.ManagedNetwork {
		group, err := d.ensureNetwork(ctx, cred)
		if err != nil {
			return providers.CreateServerResult{Outcome: providers.OutcomeRejected, ComputeNotDispatched: true}, err
		}
		cred.Groups = []string{group}
	}
	q := url.Values{"ImageId": {r.ImageID}, "InstanceType": {r.PlanID}, "MinCount": {"1"}, "MaxCount": {"1"}, "ClientToken": {d.clientToken(r.Identity)}, "UserData": {base64.StdEncoding.EncodeToString([]byte(data))}, "MetadataOptions.HttpTokens": {"required"}, "MetadataOptions.HttpEndpoint": {"enabled"}, "NetworkInterface.1.DeviceIndex": {"0"}, "NetworkInterface.1.SubnetId": {cred.Subnet}, "NetworkInterface.1.AssociatePublicIpAddress": {"true"}, "NetworkInterface.1.DeleteOnTermination": {"true"}, "BlockDeviceMapping.1.DeviceName": {img.Root}, "BlockDeviceMapping.1.Ebs.DeleteOnTermination": {"true"}, "BlockDeviceMapping.1.Ebs.VolumeType": {"gp3"}, "BlockDeviceMapping.1.Ebs.VolumeSize": {strconv.Itoa(max(12, img.RootSize()))}}
	for i, g := range cred.Groups {
		q.Set(fmt.Sprintf("NetworkInterface.1.SecurityGroupId.%d", i+1), g)
	}
	// Tag both server and root disk atomically; no unjournaled auxiliary resources.
	tags := []tag{{"Name", r.Name}, {"dob:managed", "true"}, {"dob:account", d.accountID}, {"dob:deployment", r.Identity}}
	for i, kind := range []string{"instance", "volume"} {
		prefix := fmt.Sprintf("TagSpecification.%d.", i+1)
		q.Set(prefix+"ResourceType", kind)
		for j, t := range tags {
			p := prefix + fmt.Sprintf("Tag.%d.", j+1)
			q.Set(p+"Key", t.Key)
			q.Set(p+"Value", t.Value)
		}
	}
	var out struct {
		Items []instance `xml:"instancesSet>item"`
	}
	rejected.ComputeNotDispatched = false
	if e = d.client.call(ctx, "ec2", "RunInstances", q, true, &out); e != nil {
		if cloudutil.Ambiguous(e) {
			return providers.CreateServerResult{Outcome: providers.OutcomeAmbiguous}, problem(providers.ErrorAmbiguousOutcome, "create_server", "CREATE_UNCONFIRMED", 0)
		}
		return rejected, e
	}
	if len(out.Items) != 1 || !idPattern.MatchString(out.Items[0].ID) || !strings.HasPrefix(out.Items[0].ID, "i-") {
		return providers.CreateServerResult{Outcome: providers.OutcomeAmbiguous}, problem(providers.ErrorAmbiguousOutcome, "create_server", "CREATE_ID_MISSING", 200)
	}
	return providers.CreateServerResult{ServerID: d.ref(out.Items[0].ID), OperationID: d.clientToken(r.Identity), Outcome: providers.OutcomeAccepted}, nil
}

var _ providers.Factory = Factory{}
var _ providers.ComputeDriver = (*Driver)(nil)
var _ providers.AccountReader = (*Driver)(nil)
var _ providers.CatalogReader = (*Driver)(nil)
var _ providers.InventoryReader = (*Driver)(nil)
var _ providers.ObservationReader = (*Driver)(nil)
var _ providers.FastObservationReader = (*Driver)(nil)
