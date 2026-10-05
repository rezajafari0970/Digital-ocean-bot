package upcloud

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

var currencyCode = regexp.MustCompile("^[A-Z]{3}$")

// Prices are optional catalog metadata, never an authorization/capacity signal.
// Use one bounded account-transport budget with no retries or direct fallback.
func (d *Driver) enrichPrices(ctx context.Context, catalog *providers.Catalog) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var response struct {
		Prices struct {
			Currency string                       `json:"currency"`
			Zones    []map[string]json.RawMessage `json:"zone"`
		} `json:"prices"`
	}
	if d.client.do(ctx, http.MethodGet, "/price", nil, &response) != nil {
		return
	}
	currency := response.Prices.Currency
	if currency == "" {
		account, err := d.account(ctx)
		if err != nil {
			return
		}
		var detail struct {
			Account struct {
				Currency string `json:"currency"`
			} `json:"account"`
		}
		if d.client.do(ctx, http.MethodGet, "/account/details/"+url.PathEscape(account.Username), nil, &detail) != nil {
			return
		}
		currency = detail.Account.Currency
	}
	if !currencyCode.MatchString(currency) {
		return
	}
	known := map[string]bool{}
	for _, region := range catalog.Regions {
		known[region.ID] = true
	}
	zones := map[string]map[string]json.RawMessage{}
	duplicate := map[string]bool{}
	for _, zone := range response.Prices.Zones {
		var name string
		if json.Unmarshal(zone["name"], &name) != nil || !known[name] {
			continue
		}
		if _, exists := zones[name]; exists {
			duplicate[name] = true
		}
		zones[name] = zone
	}
	for i := range catalog.Plans {
		p := &catalog.Plans[i]
		for zone, items := range zones {
			if duplicate[zone] {
				continue
			}
			var entry struct {
				Amount json.RawMessage `json:"amount"`
				Price  json.RawMessage `json:"price"`
			}
			if json.Unmarshal(items["server_plan_"+p.ID], &entry) != nil {
				continue
			}
			amount, ok := positiveDecimal(entry.Amount)
			if !ok || amount != 1 {
				continue
			}
			price, ok := positiveDecimal(entry.Price)
			if !ok {
				continue
			}
			// Official upcloud-cli getPlanCost: API units / 100; 28 days for monthly estimates.
			hourly := price / 100
			monthly := hourly * 672
			if hourly <= 0 || math.IsInf(monthly, 0) {
				continue
			}
			if p.PricesByRegion == nil {
				p.PricesByRegion = map[string]providers.PlanPrice{}
			}
			p.PricesByRegion[zone] = providers.PlanPrice{Currency: currency, Hourly: hourly, MonthlyEstimate: monthly}
		}
	}
}

func positiveDecimal(raw json.RawMessage) (float64, bool) {
	var n json.Number
	if json.Unmarshal(raw, &n) != nil {
		return 0, false
	}
	value, err := n.Float64()
	return value, err == nil && value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
