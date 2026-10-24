package aws

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func regionalFixture(r *http.Request) string {
	region := strings.TrimSuffix(strings.TrimPrefix(r.URL.Host, "ec2."), ".amazonaws.com")
	if strings.HasPrefix(r.URL.Host, "sts.") {
		return fixtureRead("GetCallerIdentity")
	}
	body := networkFixture(query(r).Get("Action"))
	if region == "us-east-1" {
		body = strings.ReplaceAll(body, "eu-west-1a", "us-east-1a")
		body = strings.ReplaceAll(body, "subnet-12345678", "subnet-87654321")
		body = strings.ReplaceAll(body, "ami-12345678", "ami-87654321")
	}
	return body
}
func regionalOpen(t *testing.T, fn transport) *regionalDriver {
	t.Helper()
	ctx := context.Background()
	c, e := credentials(ctx, testCredential(fixtureCredential))
	if e != nil {
		t.Fatal(e)
	}
	c.Networks = map[string]regionalNetwork{"us-east-1": {Subnet: "subnet-87654321", Groups: []string{"sg-12345678"}}}
	d, e := (Factory{}).Open(ctx, providers.OpenRequest{AccountID: "account-a", ExpectedExternalID: "123456789012:eu-west-1", Credentials: cloudutil.NewJSONCredential(c), HTTPClient: &http.Client{Transport: fn}})
	if e != nil {
		t.Fatal(e)
	}
	return d.(*regionalDriver)
}
func regionalResponse(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "api.pricing.us-east-1.amazonaws.com" {
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"__type":"AccessDeniedException"}`))}, nil
	}
	return response(query(r).Get("Action"), regionalFixture(r)), nil
}
func TestAWSRegionalDiscoveryPinsIndependentNetworks(t *testing.T) {
	ctx := context.Background()
	calls := 0
	req := providers.OpenRequest{AccountID: "preview", Credentials: testCredential(`{"access_key_id":"fixture-access","secret_access_key":"fixture-secret"}`), HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" {
			t.Fatal("preview mutation")
		}
		return regionalResponse(r)
	})}}
	d, e := (Factory{}).Discover(ctx, req, map[string]string{"region_2": "us-east-1"})
	if e != nil {
		t.Fatal(e)
	}
	c, e := credentials(ctx, d.Credentials)
	if e != nil || !d.Complete || !d.NetworkApproval || len(c.Networks) != 1 || c.Region != "eu-west-1" || c.Subnet != "subnet-12345678" || c.Networks["us-east-1"].Subnet != "subnet-87654321" || !c.Networks["us-east-1"].Managed {
		t.Fatal("regional network not pinned", e)
	}
	raw, _ := json.Marshal(d)
	if strings.Contains(string(raw), "fixture-secret") || strings.Contains(string(raw), "fixture-access") {
		t.Fatal("credential leaked")
	}
	if calls > 20 {
		t.Fatal("unbounded discovery", calls)
	}
	for _, selection := range []map[string]string{{"region_2": "eu-west-1"}, {"region_2": "us-east-1", "region_3": "us-east-1"}, {"region_2": "ap-east-1"}} {
		if _, e := (Factory{}).Discover(ctx, req, selection); e == nil {
			t.Fatal("duplicate or disabled region accepted")
		}
	}
}
func TestAWSRegionalCatalogAndLaunchRouting(t *testing.T) {
	var mu sync.Mutex
	posts := 0
	d := regionalOpen(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.pricing.us-east-1.amazonaws.com" {
			return regionalResponse(r)
		}
		q := query(r)
		action := q.Get("Action")
		if action == "RunInstances" {
			mu.Lock()
			posts++
			mu.Unlock()
			if r.URL.Host != "ec2.us-east-1.amazonaws.com" || q.Get("NetworkInterface.1.SubnetId") != "subnet-87654321" || q.Get("ImageId") != "ami-87654321" || q.Get("BlockDeviceMapping.1.Ebs.VolumeSize") != "12" {
				t.Error("cross-region launch routing")
			}
			return response(action, `<instancesSet><item><instanceId>i-12345678</instanceId></item></instancesSet>`), nil
		}
		return regionalResponse(r)
	})
	a, e := d.Account(context.Background())
	if e != nil || a.ID != "123456789012:eu-west-1" {
		t.Fatal("legacy identity changed", e)
	}
	c, e := d.Catalog(context.Background())
	if e != nil || len(c.Regions) != 2 || len(c.Plans) != 1 || len(c.Plans[0].AvailableRegions) != 2 || len(c.Images) != 1 || c.Images[0].IDsByRegion["us-east-1"] != "ami-87654321" || c.Plans[0].DiskGB != 0 {
		t.Fatal(c, e)
	}
	req := providers.CreateServerRequest{Name: "fixture", Identity: "deployment-fixed", RegionID: "us-east-1", PlanID: "t3.micro", ImageID: "ami-87654321", SSHAuthorizedKeys: []string{publicKey(t)}}
	created, e := d.CreateServer(context.Background(), req)
	if e != nil || created.ServerID != "us-east-1:i-12345678" || posts != 1 {
		t.Fatal(created, e, posts)
	}
	req.RegionID = "us-west-2"
	created, e = d.CreateServer(context.Background(), req)
	if e == nil || !created.ComputeNotDispatched || posts != 1 {
		t.Fatal("unconfigured region dispatched")
	}
	if _, e := d.GetServer(context.Background(), "us-west-2:i-12345678"); e == nil {
		t.Fatal("unconfigured resource accepted")
	}
}
func TestAWSRegionalInventoryNeverReturnsPartialSuccess(t *testing.T) {
	d := regionalOpen(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Host, "ec2.us-east-1.") {
			return nil, errors.New("regional connection failed")
		}
		return regionalResponse(r)
	})
	if xs, e := d.ListServers(context.Background()); e == nil || xs != nil {
		t.Fatal("partial inventory accepted")
	}
	if xs, e := d.FindServerByIdentity(context.Background(), "deployment-fixed"); e == nil || xs != nil {
		t.Fatal("partial identity search accepted")
	}
	if _, e := d.Capacity(context.Background()); e == nil {
		t.Fatal("incomplete capacity accepted")
	}
	if _, e := d.ObserveFast(context.Background()); e == nil {
		t.Fatal("incomplete observation accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := d.ListServers(ctx); e == nil {
		t.Fatal("cancelled read succeeded")
	}
}
func TestAWSRegionalCredentialsRejectScopeChanges(t *testing.T) {
	c, e := credentials(context.Background(), testCredential(fixtureCredential))
	if e != nil {
		t.Fatal(e)
	}
	c.Networks = map[string]regionalNetwork{"us-east-1": {Subnet: "subnet-87654321", Managed: true}}
	source := regionalCredential{cloudutil.NewJSONCredential(c), "eu-west-1", "us-east-1", c.Networks["us-east-1"]}
	if _, e := source.Get(context.Background()); e != nil {
		t.Fatal(e)
	}
	c.Networks["us-east-1"] = regionalNetwork{Subnet: "subnet-11111111", Managed: true}
	source.source = cloudutil.NewJSONCredential(c)
	if _, e := source.Get(context.Background()); e == nil {
		t.Fatal("network scope silently changed")
	}
	c.Networks = map[string]regionalNetwork{"eu-west-1": {Subnet: "subnet-87654321", Managed: true}}
	if _, e := credentials(context.Background(), cloudutil.NewJSONCredential(c)); e == nil {
		t.Fatal("anchor region overwritten")
	}
}
func TestAWSRegionalCleanupRejectsUnknownJournalBeforeIO(t *testing.T) {
	calls := 0
	d := regionalOpen(t, func(r *http.Request) (*http.Response, error) { calls++; return regionalResponse(r) })
	j := &networkJournal{exists: true, record: providers.AccountResource{Scope: "us-west-2:vpc-12345678", Name: "unknown"}}
	d.resources = j
	for region, r := range d.drivers {
		r.resources = regionalResources{j, region}
	}
	if e := d.CleanupAccountResources(context.Background()); e == nil || calls != 0 {
		t.Fatal("unknown regional resource passed cleanup", e, calls)
	}
	j.record = providers.AccountResource{Scope: "us-east-1:vpc-12345678", Name: d.drivers["us-east-1"].networkName("vpc-12345678"), Complete: true}
	if e := d.CleanupAccountResources(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := (regionalResources{j, "eu-west-1"}).WithLock(context.Background(), "us-east-1:vpc-12345678", "x", func(providers.AccountResourceSession) error {
		t.Fatal("wrong-region journal lock acquired")
		return nil
	}); e == nil {
		t.Fatal("wrong scope accepted")
	}
}

type regionalTestJournal map[string]*networkJournal

func (j regionalTestJournal) List(ctx context.Context) ([]providers.AccountResource, error) {
	out := []providers.AccountResource{}
	for _, n := range j {
		rows, e := n.List(ctx)
		if e != nil {
			return nil, e
		}
		out = append(out, rows...)
	}
	return out, nil
}
func (j regionalTestJournal) WithLock(ctx context.Context, scope, name string, fn func(providers.AccountResourceSession) error) error {
	n := j[scope]
	if n == nil || n.record.Name != name {
		return errors.New("wrong scope")
	}
	return n.WithLock(ctx, scope, name, fn)
}
func TestAWSRegionalCleanupResumesEachOwnedNetwork(t *testing.T) {
	j := regionalTestJournal{}
	removed := map[string]bool{}
	deletes := map[string]int{}
	d := regionalOpen(t, func(r *http.Request) (*http.Response, error) {
		q := query(r)
		a := q.Get("Action")
		region := strings.TrimSuffix(strings.TrimPrefix(r.URL.Host, "ec2."), ".amazonaws.com")
		scope := region + ":vpc-12345678"
		switch a {
		case "DescribeSecurityGroups":
			n := j[scope]
			if n == nil {
				t.Fatal("unconfigured scope requested")
			}
			if removed[region] {
				return response(a, "<securityGroupInfo/>"), nil
			}
			return response(a, groupXML(n.record.Name, n.record.Nonce, true)), nil
		case "DescribeNetworkInterfaces":
			return response(a, "<networkInterfaceSet/>"), nil
		case "DeleteSecurityGroup":
			if NetworkDeletionTarget(r) != region+":sg-12345678" || j[scope] == nil {
				t.Fatal("wrong regional delete target")
			}
			deletes[region]++
			removed[region] = true
			return response(a, "<return>true</return>"), nil
		}
		return regionalResponse(r)
	})
	d.resources = j
	for region, r := range d.drivers {
		scope := region + ":vpc-12345678"
		j[scope] = &networkJournal{exists: true, stable: true, record: providers.AccountResource{Scope: scope, Name: r.networkName("vpc-12345678"), Nonce: "durable-" + region, ID: "sg-12345678", Attempted: true}}
		r.resources = regionalResources{j, region}
	}
	// Successful delete must still wait for a subsequent stable absence read.
	for i := 0; i < 2; i++ {
		if e := d.CleanupAccountResources(context.Background()); e == nil {
			t.Fatal("delete completed without absence confirmation")
		}
	}
	if e := d.CleanupAccountResources(context.Background()); e != nil {
		t.Fatal(e)
	}
	for _, region := range d.regions {
		if deletes[region] != 1 || !j[region+":vpc-12345678"].record.Complete {
			t.Fatal("regional cleanup not reconciled", region, deletes)
		}
	}
}
