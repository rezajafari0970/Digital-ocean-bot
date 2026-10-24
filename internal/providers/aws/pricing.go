package aws

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Optional display metadata from the official AWS Price List Query API.
// These are Linux shared-tenancy On-Demand compute prices, never an invoice
// or permission to spend. EBS, public IPv4, transfer, credits and tax are extra.
const pricingEndpoint = "https://api.pricing.us-east-1.amazonaws.com/"

type priceFilter struct{ Field, Type, Value string }
type priceProduct struct {
	Product struct {
		SKU           string
		ProductFamily string
		Attributes    map[string]string
	}
	Terms struct {
		OnDemand map[string]struct {
			EffectiveDate   string
			PriceDimensions map[string]struct {
				Unit, BeginRange, EndRange string
				PricePerUnit               map[string]string
			}
		}
	}
}

func computePrice(raw, region string, now time.Time) (string, float64, bool) {
	var p priceProduct
	if json.Unmarshal([]byte(raw), &p) != nil || p.Product.ProductFamily != "Compute Instance" {
		return "", 0, false
	}
	a := p.Product.Attributes
	for k, v := range map[string]string{"regionCode": region, "operatingSystem": "Linux", "tenancy": "Shared", "preInstalledSw": "NA", "capacitystatus": "Used", "operation": "RunInstances"} {
		if a[k] != v {
			return "", 0, false
		}
	}
	id := a["instanceType"]
	if id == "" || len(p.Terms.OnDemand) != 1 {
		return "", 0, false
	}
	for _, term := range p.Terms.OnDemand {
		effective, e := time.Parse(time.RFC3339, term.EffectiveDate)
		if e != nil || effective.After(now) || len(term.PriceDimensions) != 1 {
			return "", 0, false
		}
		for _, v := range term.PriceDimensions {
			if v.Unit != "Hrs" || v.BeginRange != "0" || v.EndRange != "Inf" {
				return "", 0, false
			}
			hourly, e := strconv.ParseFloat(v.PricePerUnit["USD"], 64)
			if e != nil || hourly <= 0 || math.IsNaN(hourly) || math.IsInf(hourly, 0) {
				return "", 0, false
			}
			return id, hourly, true
		}
	}
	return "", 0, false
}
func (d *Driver) loadPrices(ctx context.Context, plans []providers.Plan) providers.CatalogPricing {
	meta := providers.CatalogPricing{Status: "unavailable", Basis: "linux_shared_ondemand_compute_only_730h", Currency: "USD", Source: pricingEndpoint}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	cred, e := signingCredentials(ctx, d.client.credentials)
	if e != nil {
		return meta
	}
	cred.Region = "us-east-1" // Pricing endpoint region is independent of product region.
	filters := []priceFilter{}
	for _, kv := range [][2]string{{"regionCode", d.client.region}, {"productFamily", "Compute Instance"}, {"operatingSystem", "Linux"}, {"tenancy", "Shared"}, {"preInstalledSw", "NA"}, {"capacitystatus", "Used"}, {"operation", "RunInstances"}} {
		filters = append(filters, priceFilter{kv[0], "TERM_MATCH", kv[1]})
	}
	prices := map[string]float64{}
	conflicts := map[string]bool{}
	tokens := map[string]bool{}
	next := ""
	remaining := int64(16 << 20)
	now := time.Now().UTC()
	// Exhaust all pages before publishing prices. A repeated/missing token or
	// failed page cannot turn partial evidence into an apparently complete rate.
	for page := 0; page < 20; page++ {
		payload := struct {
			ServiceCode, FormatVersion string
			MaxResults                 int
			Filters                    []priceFilter
			NextToken                  string `json:",omitempty"`
		}{"AmazonEC2", "aws_v1", 100, filters, next}
		body, _ := json.Marshal(payload)
		request, _ := http.NewRequestWithContext(ctx, http.MethodPost, pricingEndpoint, strings.NewReader(string(body)))
		request.Header.Set("Content-Type", "application/x-amz-json-1.1")
		request.Header.Set("X-Amz-Target", "AWSPriceListService.GetProducts")
		sign(request, string(body), cred, "pricing", now)
		response, err := d.client.http.Do(request)
		if err != nil {
			return meta
		}
		limit := min(remaining, int64(4<<20))
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, limit+1))
		response.Body.Close()
		remaining -= int64(len(raw))
		if readErr != nil || int64(len(raw)) > limit || remaining < 0 {
			return meta
		}
		if response.StatusCode != 200 {
			var failure struct {
				Type string `json:"__type"`
			}
			_ = json.Unmarshal(raw, &failure)
			if strings.HasSuffix(failure.Type, "AccessDeniedException") {
				meta.Status = "permission_required"
			}
			if response.StatusCode == 429 || strings.HasSuffix(failure.Type, "ThrottlingException") {
				meta.Status = "rate_limited"
			}
			return meta
		}
		var result struct {
			FormatVersion string
			PriceList     *[]string
			NextToken     string
		}
		if json.Unmarshal(raw, &result) != nil || result.PriceList == nil || result.FormatVersion != "aws_v1" {
			return meta
		}
		for _, product := range *result.PriceList {
			if id, price, ok := computePrice(product, d.client.region, now); ok {
				if old, seen := prices[id]; seen && old != price {
					conflicts[id] = true
				}
				prices[id] = price
			}
		}
		next = result.NextToken
		if next != "" {
			if tokens[next] || len(next) > 16384 {
				return meta
			}
			tokens[next] = true
			continue
		}
		count := 0
		for i := range plans {
			p := &plans[i]
			if price, ok := prices[p.ID]; ok && !conflicts[p.ID] {
				p.PricesByRegion = map[string]providers.PlanPrice{d.client.region: {Currency: "USD", Hourly: price, MonthlyEstimate: price * 730, Complete: false, Scope: "compute_only"}}
				count++
			}
		}
		if count > 0 {
			meta.Status = "partial"
			meta.ObservedAt = now
			if count == len(plans) {
				meta.Status = "available"
			}
		}
		return meta
	}
	return meta
}
