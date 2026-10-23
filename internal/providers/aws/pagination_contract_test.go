package aws

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

// API contracts, independently checked against the AWS EC2 API reference.
// RouteTables and InstanceTypes have a 100-item cap; the other operations below
// accept 1000. A uniform EC2 page size causes live credential discovery to fail.
func TestAWSPaginationOperationContracts(t *testing.T) {
	for _, tc := range []struct {
		action string
		max    int
	}{
		{"DescribeRouteTables", 100}, {"DescribeInstanceTypes", 100},
		{"DescribeSubnets", 1000}, {"DescribeNetworkAcls", 1000},
		{"DescribeInternetGateways", 1000}, {"DescribeSecurityGroups", 1000},
		{"DescribeNetworkInterfaces", 1000}, {"DescribeInstances", 1000},
		{"DescribeImages", 1000}, {"DescribeInstanceTypeOfferings", 1000},
	} {
		t.Run(tc.action, func(t *testing.T) {
			calls := 0
			token := "next+/=page"
			d := openTest(t, func(r *http.Request) (*http.Response, error) {
				calls++
				q := r.URL.Query()
				n, e := strconv.Atoi(q.Get("MaxResults"))
				if e != nil || n < 5 || n > tc.max {
					t.Fatalf("%s MaxResults=%q outside official [5,%d]", tc.action, q.Get("MaxResults"), tc.max)
				}
				if r.Method != "GET" || q.Get("Action") != tc.action || q.Get("Filter.1.Value.1") != "preserve-me" {
					t.Fatal("pagination changed read intent")
				}
				switch calls {
				case 1:
					if q.Get("NextToken") != "" {
						t.Fatal("first page has token")
					}
					return response(tc.action, `<items><item><id>one</id></item></items><nextToken>`+token+`</nextToken>`), nil
				case 2:
					if q.Get("NextToken") != token {
						t.Fatal("continuation token changed")
					}
					return response(tc.action, `<items><item><id>two</id></item></items>`), nil
				default:
					t.Fatal("unexpected extra page")
					return nil, nil
				}
			})
			type item struct {
				ID string `xml:"id"`
			}
			rows, e := pages(context.Background(), d.client, tc.action, "items", url.Values{"Filter.1.Name": {"tag:test"}, "Filter.1.Value.1": {"preserve-me"}}, func(x item) string { return x.ID })
			if e != nil || len(rows) != 2 || calls != 2 || rows[0].ID != "one" || rows[1].ID != "two" {
				t.Fatalf("incomplete pagination rows=%v calls=%d err=%v", rows, calls, e)
			}
		})
	}
}

func TestAWSDiscoveryFollowsRouteTablePageWithValidLimit(t *testing.T) {
	calls := map[string]int{}
	req := providers.OpenRequest{AccountID: "preview", Credentials: testCredential(`{"access_key_id":"fixture-access","secret_access_key":"fixture-secret"}`), HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatal("discovery mutated AWS")
		}
		q := query(r)
		a := q.Get("Action")
		calls[a]++
		body := networkFixture(a)
		if a == "DescribeRouteTables" {
			n, e := strconv.Atoi(q.Get("MaxResults"))
			if e != nil || n < 5 || n > 100 {
				return &http.Response{StatusCode: 400, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`<Response><Errors><Error><Code>InvalidParameterValue</Code><Message>MaxResults must be between 5 and 100</Message></Error></Errors></Response>`))}, nil
			}
			if calls[a] == 1 {
				if q.Get("NextToken") != "" {
					t.Fatal("unexpected initial token")
				}
				body = strings.ReplaceAll(strings.ReplaceAll(networkRoutes, "rtb-12345678", "rtb-87654321"), "vpc-12345678", "vpc-87654321") + `<nextToken>route+/=two</nextToken>`
			} else if calls[a] == 2 {
				if q.Get("NextToken") != "route+/=two" {
					t.Fatal("route page token lost")
				}
			} else {
				t.Fatal("duplicate route read")
			}
		}
		return response(a, body), nil
	})}}
	discovery, e := (Factory{}).Discover(context.Background(), req, nil)
	if e != nil || !discovery.Complete || discovery.Resolved["subnet_id"] != "subnet-12345678" || discovery.Resolved["managed_network"] != true || calls["DescribeRouteTables"] != 2 {
		t.Fatalf("live-contract discovery failed: complete=%v resolved=%v route_calls=%d err=%v", discovery.Complete, discovery.Resolved, calls["DescribeRouteTables"], e)
	}
	for _, action := range []string{"DescribeRegions", "GetCallerIdentity", "DescribeSubnets", "DescribeNetworkAcls", "DescribeInternetGateways"} {
		if calls[action] != 1 {
			t.Fatalf("unexpected %s count %d", action, calls[action])
		}
	}
}

func TestAWSUnregisteredPaginationFailsBeforeRequest(t *testing.T) {
	calls := 0
	d := openTest(t, func(r *http.Request) (*http.Response, error) {
		calls++
		return response("DescribeUnregistered", `<items/>`), nil
	})
	type item struct {
		ID string `xml:"id"`
	}
	rows, e := pages(context.Background(), d.client, "DescribeUnregistered", "items", nil, func(x item) string { return x.ID })
	if e == nil || rows != nil || calls != 0 {
		t.Fatalf("unregistered pagination was sent: rows=%v calls=%d err=%v", rows, calls, e)
	}
}
