package aws

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"strings"
	"testing"
)

const networkRoutes = `<routeTableSet><item><routeTableId>rtb-12345678</routeTableId><vpcId>vpc-12345678</vpcId><associationSet><item><main>true</main><associationState><state>associated</state></associationState></item></associationSet><routeSet><item><destinationCidrBlock>0.0.0.0/0</destinationCidrBlock><gatewayId>igw-12345678</gatewayId><state>active</state></item></routeSet></item></routeTableSet>`
const networkACLs = `<networkAclSet><item><networkAclId>acl-12345678</networkAclId><vpcId>vpc-12345678</vpcId><associationSet><item><subnetId>subnet-12345678</subnetId></item></associationSet><entrySet><item><ruleNumber>100</ruleNumber><protocol>-1</protocol><ruleAction>allow</ruleAction><egress>false</egress><cidrBlock>0.0.0.0/0</cidrBlock></item><item><ruleNumber>100</ruleNumber><protocol>-1</protocol><ruleAction>allow</ruleAction><egress>true</egress><cidrBlock>0.0.0.0/0</cidrBlock></item></entrySet></item></networkAclSet>`

func networkFixture(action string) string {
	switch action {
	case "DescribeRegions":
		return `<regionInfo><item><regionName>eu-west-1</regionName><optInStatus>opt-in-not-required</optInStatus></item><item><regionName>us-east-1</regionName><optInStatus>opt-in-not-required</optInStatus></item></regionInfo>`
	case "DescribeSubnets":
		return strings.Replace(fixtureRead(action), "</state>", "</state><availableIpAddressCount>200</availableIpAddressCount><availabilityZone>eu-west-1a</availabilityZone><defaultForAz>true</defaultForAz>", 1)
	case "DescribeRouteTables":
		return networkRoutes
	case "DescribeNetworkAcls":
		return networkACLs
	case "DescribeInternetGateways":
		return `<internetGatewaySet><item><internetGatewayId>igw-12345678</internetGatewayId><attachmentSet><item><vpcId>vpc-12345678</vpcId><state>available</state></item></attachmentSet></item></internetGatewaySet>`
	}
	return fixtureRead(action)
}
func TestAWSAutomaticDiscoveryReadOnlyAndPinned(t *testing.T) {
	req := providers.OpenRequest{AccountID: "preview", Credentials: testCredential(`{"access_key_id":"fixture-access","secret_access_key":"fixture-secret"}`), HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatal("discovery mutation")
		}
		q := query(r)
		return response(q.Get("Action"), networkFixture(q.Get("Action"))), nil
	})}}
	d, e := (Factory{}).Discover(context.Background(), req, nil)
	if e != nil || !d.Complete || d.Resolved["managed_network"] != true || d.Resolved["region"] != "eu-west-1" || d.Resolved["subnet_id"] != "subnet-12345678" {
		t.Fatal(d, e)
	}
	c, e := credentials(context.Background(), d.Credentials)
	if e != nil || !c.ManagedNetwork || c.SecretKey != "fixture-secret" {
		t.Fatal("invalid pinned credentials", e)
	}
	b, _ := json.Marshal(d)
	if strings.Contains(string(b), "fixture-") {
		t.Fatal("secret echoed")
	}
	d, e = (Factory{}).Discover(context.Background(), req, map[string]string{"region": "us-east-1"})
	if e != nil || d.Resolved["region"] != "us-east-1" {
		t.Fatal(d, e)
	}
	req.Credentials = testCredential(`{"access_key_id":"ASIAfixture","secret_access_key":"fixture-secret"}`)
	if _, e = (Factory{}).Discover(context.Background(), req, nil); e == nil || !strings.Contains(e.Error(), "TEMPORARY_SESSION_TOKEN_REQUIRED") {
		t.Fatal(e)
	}
}
func TestAWSPublicNetworkRejectsBlockedAndPartialReads(t *testing.T) {
	for _, bad := range []string{"blackhole", "wrong_gateway", "acl_deny_first", "no_return_path", "explicit_private", "missing_collection", "pagination_loop"} {
		t.Run(bad, func(t *testing.T) {
			d := openTest(t, func(r *http.Request) (*http.Response, error) {
				a := query(r).Get("Action")
				body := networkFixture(a)
				switch bad {
				case "blackhole":
					if a == "DescribeRouteTables" {
						body = strings.ReplaceAll(body, ">active<", ">blackhole<")
					}
				case "wrong_gateway":
					if a == "DescribeInternetGateways" {
						body = strings.ReplaceAll(body, "vpc-12345678", "vpc-87654321")
					}
				case "acl_deny_first":
					if a == "DescribeNetworkAcls" {
						body = strings.Replace(body, "<entrySet>", `<entrySet><item><ruleNumber>50</ruleNumber><protocol>6</protocol><ruleAction>deny</ruleAction><egress>false</egress><cidrBlock>192.0.2.0/24</cidrBlock></item>`, 1)
					}
				case "no_return_path":
					if a == "DescribeNetworkAcls" {
						body = strings.ReplaceAll(body, "<egress>true</egress>", "<egress>false</egress>")
					}
				case "explicit_private":
					if a == "DescribeRouteTables" {
						body = strings.Replace(body, "</routeTableSet>", `<item><routeTableId>rtb-87654321</routeTableId><vpcId>vpc-12345678</vpcId><associationSet><item><subnetId>subnet-12345678</subnetId></item></associationSet><routeSet/></item></routeTableSet>`, 1)
					}
				case "missing_collection":
					if a == "DescribeRouteTables" {
						body = ""
					}
				case "pagination_loop":
					if a == "DescribeRouteTables" {
						body += `<nextToken>same-token</nextToken>`
					}
				}
				return response(a, body), nil
			})
			subs, e := d.publicSubnets(context.Background(), "")
			if e == nil && len(subs) > 0 {
				t.Fatal("unsafe network accepted", bad)
			}
		})
	}
}

func TestAWSDiscoverySelectionOverridesStaleSubnet(t *testing.T) {
	req := providers.OpenRequest{AccountID: "preview", Credentials: testCredential(`{"access_key_id":"fixture-access","secret_access_key":"fixture-secret","subnet_id":"subnet-87654321"}`), HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		q := query(r)
		if q.Get("Action") == "DescribeSubnets" && q.Get("Filter.1.Name") != "" {
			t.Fatal("discovery pinned to stale override")
		}
		return response(q.Get("Action"), networkFixture(q.Get("Action"))), nil
	})}}
	d, e := (Factory{}).Discover(context.Background(), req, map[string]string{"subnet_id": "subnet-12345678"})
	if e != nil || !d.Complete || d.Resolved["subnet_id"] != "subnet-12345678" {
		t.Fatal(d, e)
	}
}
