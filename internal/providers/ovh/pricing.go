package ovh

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

// Pricing is optional display evidence. It never controls authentication,
// scheduling or retries of compute mutations.
type catalogOffer struct {
	Code        string `json:"planCode"`
	Product     string `json:"product"`
	PricingType string `json:"pricingType"`
	Pricings    []struct {
		Price           *int64 `json:"price"`
		Interval        int    `json:"interval"`
		Unit            string `json:"intervalUnit"`
		Mode            string `json:"mode"`
		Type            string `json:"type"`
		Commitment      int    `json:"commitment"`
		Phase           int    `json:"phase"`
		MustBeCompleted bool   `json:"mustBeCompleted"`
		Quantity        struct {
			Min int  `json:"min"`
			Max *int `json:"max"`
		} `json:"quantity"`
	} `json:"pricings"`
	Addons []struct {
		Name  string   `json:"name"`
		Codes []string `json:"addons"`
	} `json:"addonFamilies"`
}
type priceCatalog struct {
	ID     int64 `json:"catalogId"`
	Locale struct {
		Currency   string `json:"currencyCode"`
		Subsidiary string `json:"subsidiary"`
	} `json:"locale"`
	Plans  []catalogOffer `json:"plans"`
	Addons []catalogOffer `json:"addons"`
}
type priceEvidence struct {
	meta   providers.CatalogPricing
	offers map[string]catalogOffer
}

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)
var subsidiaryCode = regexp.MustCompile(`^[A-Z]{2,4}$`)

func (d *Driver) pricing(ctx context.Context) priceEvidence {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out := priceEvidence{meta: providers.CatalogPricing{Status: "unavailable", Basis: "public_reference", TaxIncluded: false}}
	cred, err := signingCredentials(ctx, d.client.credentials)
	if err != nil {
		return out
	}
	subsidiary := cred.PricingSubsidiary
	if subsidiary != "" && !subsidiaryCode.MatchString(subsidiary) {
		return out
	}
	if subsidiary != "" {
		out.meta.Basis = "selected_market"
	} else {
		meCtx, stop := context.WithTimeout(ctx, 3*time.Second)
		var me struct {
			Subsidiary string `json:"ovhSubsidiary"`
		}
		err = d.client.do(meCtx, http.MethodGet, "/me", nil, &me)
		stop()
		if err == nil && subsidiaryCode.MatchString(me.Subsidiary) {
			subsidiary = me.Subsidiary
			out.meta.Basis = "account_market"
		} else {
			subsidiary = map[string]string{"ovh-eu": "FR", "ovh-ca": "CA", "ovh-us": "US"}[d.client.endpoint]
		}
	}
	out.meta.Subsidiary = subsidiary
	out.meta.Source = endpoints[d.client.endpoint] + "/order/catalog/public/cloud?ovhSubsidiary=" + url.QueryEscape(subsidiary)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, out.meta.Source, nil)
	if err != nil {
		return out
	}
	req.Header.Set("Accept", "application/json")
	// This public endpoint must receive no OVH credential/signature headers.
	resp, err := d.client.http.Do(req)
	if err != nil {
		return out
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (24<<20)+1))
	if err != nil || len(raw) > 24<<20 {
		return out
	}
	var cat priceCatalog
	if json.Unmarshal(raw, &cat) != nil || cat.ID <= 0 || cat.Locale.Subsidiary != subsidiary || !currencyCode.MatchString(cat.Locale.Currency) || len(cat.Addons) == 0 {
		return out
	}
	out.offers = make(map[string]catalogOffer, len(cat.Addons))
	for _, offer := range append(cat.Plans, cat.Addons...) {
		if offer.Code == "" {
			continue
		}
		if _, duplicate := out.offers[offer.Code]; duplicate {
			out.offers = nil
			return out
		}
		out.offers[offer.Code] = offer
	}
	out.meta.Currency = cat.Locale.Currency
	out.meta.CatalogID = cat.ID
	out.meta.ObservedAt = time.Now().UTC()
	out.meta.Status = "available"
	return out
}

// Exact integer microcents are converted once; formattedPrice is rounded by
// OVH and must never be parsed. No name-based guessed plan-code fallback.
func (p priceEvidence) rate(code string) (int64, bool) {
	offer, ok := p.offers[code]
	if !ok || offer.PricingType != "consumption" {
		return 0, false
	}
	var value int64
	found := false
	for _, r := range offer.Pricings {
		if r.Mode != "default" || r.Type != "consumption" || r.Commitment != 0 || r.Phase != 0 || r.MustBeCompleted {
			continue
		}
		if r.Unit != "hour" || r.Interval != 1 || r.Quantity.Min != 1 || r.Quantity.Max != nil || r.Price == nil || *r.Price < 0 {
			continue
		}
		if found {
			return 0, false
		}
		value = *r.Price
		found = true
	}
	return value, found
}

func (p priceEvidence) flavor(f flavorData) (providers.PlanPrice, bool) {
	compute, ok := p.rate(f.PlanCodes.Hourly)
	if !ok {
		return providers.PlanPrice{}, false
	}
	components := []providers.PriceComponent{{Name: "Compute", PlanCode: f.PlanCodes.Hourly, Quantity: 1, Hourly: float64(compute) / 1e8}}
	total := float64(compute) / 1e8
	complete := true
	for _, family := range p.offers[f.PlanCodes.Hourly].Addons {
		if family.Name != "local-storage" {
			continue
		}
		if len(family.Codes) != 1 {
			complete = false
			continue
		}
		rate, valid := p.rate(family.Codes[0])
		if !valid {
			complete = false
			continue
		}
		price := float64(rate) / 1e8 * float64(f.Disk)
		components = append(components, providers.PriceComponent{Name: "Local storage", PlanCode: family.Codes[0], Quantity: f.Disk, Hourly: price})
		total += price
	}
	if f.PlanCodes.License != "" {
		rate, valid := p.rate(f.PlanCodes.License)
		if !valid {
			complete = false
		} else {
			price := float64(rate) / 1e8 * float64(f.CPU)
			components = append(components, providers.PriceComponent{Name: "OS license", PlanCode: f.PlanCodes.License, Quantity: f.CPU, Hourly: price})
			total += price
		}
	}
	// Public IP billing differs by generation, network and region. It is excluded
	// explicitly, never substituted with a floating-IP or guessed regional code.
	return providers.PlanPrice{Currency: p.meta.Currency, Hourly: total, MonthlyEstimate: total * 730, Components: components, Complete: complete, Scope: "compute_and_catalog_disk_excluding_network"}, true
}
