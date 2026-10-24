package aws

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"
)

const priceFixture = `{"product":{"sku":"fixture","productFamily":"Compute Instance","attributes":{"instanceType":"t3.micro","regionCode":"eu-west-1","operatingSystem":"Linux","tenancy":"Shared","preInstalledSw":"NA","capacitystatus":"Used","operation":"RunInstances"}},"terms":{"OnDemand":{"term":{"effectiveDate":"2020-01-01T00:00:00Z","priceDimensions":{"rate":{"unit":"Hrs","beginRange":"0","endRange":"Inf","pricePerUnit":{"USD":"0.0114"}}}}}}}`

func TestAWSPriceEvidenceRejectsWrongProductAndTerms(t *testing.T) {
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	if id, p, ok := computePrice(priceFixture, "eu-west-1", now); !ok || id != "t3.micro" || p != .0114 {
		t.Fatal(id, p, ok)
	}
	for _, pair := range [][2]string{{"eu-west-1", "us-east-1"}, {"Linux", "Windows"}, {"Shared", "Dedicated"}, {"Used", "UnusedCapacityReservation"}, {"RunInstances", "RunInstances:0002"}, {"OnDemand", "Reserved"}, {"Hrs", "Quantity"}, {`"beginRange":"0"`, `"beginRange":"1"`}, {`"endRange":"Inf"`, `"endRange":"100"`}, {"0.0114", "NaN"}, {"0.0114", "-1"}, {"2020-01-01", "2099-01-01"}} {
		if _, _, ok := computePrice(strings.Replace(priceFixture, pair[0], pair[1], 1), "eu-west-1", now); ok {
			t.Fatal("ineligible pricing accepted", pair)
		}
	}
}
func TestAWSPricingPaginationSigningAndOptionalFailure(t *testing.T) {
	for _, mode := range []string{"success", "denied", "loop", "malformed", "failed_second_page", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			d := openTest(t, func(*http.Request) (*http.Response, error) { t.Fatal("unexpected EC2 read"); return nil, nil })
			d.client.http = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != pricingEndpoint || r.Method != "POST" || r.Header.Get("X-Amz-Target") != "AWSPriceListService.GetProducts" || !strings.Contains(r.Header.Get("Authorization"), "/us-east-1/pricing/aws4_request") || r.Header.Get("X-Amz-Security-Token") != "fixture-session" {
					t.Fatal("wrong pricing endpoint or signature")
				}
				var q struct {
					NextToken  string
					MaxResults int
					Filters    []priceFilter
				}
				if json.NewDecoder(r.Body).Decode(&q) != nil || q.MaxResults != 100 {
					t.Fatal("invalid pricing request")
				}
				found := false
				for _, f := range q.Filters {
					if f.Field == "regionCode" && f.Value == "eu-west-1" {
						found = true
					}
				}
				if !found {
					t.Fatal("endpoint region confused with product region")
				}
				status := 200
				var raw []byte
				switch {
				case mode == "denied":
					status = 403
					raw = []byte(`{"__type":"AccessDeniedException"}`)
				case mode == "malformed":
					raw = []byte(`{"FormatVersion":"aws_v1"}`)
				case mode == "failed_second_page" && calls == 2:
					status = 503
					raw = []byte(`{}`)
				default:
					next := ""
					products := []string{priceFixture}
					if calls == 1 || mode == "loop" {
						next = "next+/=token"
					}
					if calls == 2 && q.NextToken != "next+/=token" {
						t.Fatal("pagination token lost")
					}
					if calls > 2 {
						t.Fatal("unbounded pagination")
					}
					if mode == "conflict" && calls == 2 {
						products = []string{strings.Replace(priceFixture, "0.0114", "0.023", 1)}
					}
					raw, _ = json.Marshal(map[string]any{"FormatVersion": "aws_v1", "NextToken": next, "PriceList": products})
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
			})}
			plans := []providers.Plan{{ID: "t3.micro"}}
			meta := d.loadPrices(context.Background(), plans)
			if mode == "success" {
				p := plans[0].PricesByRegion["eu-west-1"]
				if meta.Status != "available" || p.Hourly != .0114 || math.Abs(p.MonthlyEstimate-.0114*730) > 1e-10 || p.Scope != "compute_only" || p.Complete || calls != 2 {
					t.Fatal(meta, p, calls)
				}
			} else {
				if len(plans[0].PricesByRegion) != 0 {
					t.Fatal("partial or ambiguous price published")
				}
				if mode == "denied" && meta.Status != "permission_required" {
					t.Fatal(meta)
				}
			}
		})
	}
}
