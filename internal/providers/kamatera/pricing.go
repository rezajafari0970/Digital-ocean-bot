package kamatera

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"golang.org/x/net/html"
)

const pricingURL = "https://www.kamatera.com/pricing/"
const publicPriceScope = "kamatera-public-base-hourly-v1"
const maxPricingBytes = 2 << 20

// The official calculator's component prices are monthly equivalents. Its
// powered-on hourly estimate divides by 730. CloudCLI creates hourly servers;
// it clears monthlypackage in hourly mode, so traffic is NOT included here.
// Sources: www.kamatera.com/pricing/ (KAMATERA_DATA + calculation.js),
// Kamatera/cloudcli-server/app/Cloudcli/ProxyServerPostProcessingMethods.php.
// These are public base estimates, never account quotations or monthly caps.
type tariffOption struct {
	Value string   `json:"value"`
	Price *float64 `json:"price"`
}
type tariffImage struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Price   *float64 `json:"price"`
	Regions []string `json:"datacenters"`
}
type publicTariff struct {
	CPU      []tariffOption            `json:"cpuOptions"`
	RAM      map[string][]tariffOption `json:"ramMB"`
	Disk     []tariffOption            `json:"diskGB"`
	Regions  map[string][]tariffOption `json:"netPck"`
	Images   []tariffImage             `json:"osList"`
	ipv4     float64
	observed time.Time
}

func (d *Driver) publicTariff(ctx context.Context) (*publicTariff, error) {
	budget := 12 * time.Second
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline)-2*time.Second < budget {
		budget = time.Until(deadline) - 2*time.Second
	}
	if budget <= 0 {
		return nil, errors.New("no pricing time budget")
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	for attempt := 0; ; attempt++ {
		p, retry, err := d.publicTariffAttempt(ctx)
		if err == nil || !retry || attempt == 1 || ctx.Err() != nil {
			return p, err
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, errors.New("public pricing unavailable")
		case <-timer.C:
		}
	}
}

func (d *Driver) publicTariffAttempt(parent context.Context) (*publicTariff, bool, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, pricingURL, nil)
	req.Header.Set("Accept", "text/html")
	// Same route as the account, with no provider credentials, cookies or redirects.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; DatacenterBotCatalog/1.0)")
	client := *d.client.http
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, true, errors.New("public pricing unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode >= 500 && resp.StatusCode <= 599, errors.New("public pricing unavailable")
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxPricingBytes+1))
	if err != nil {
		return nil, true, errors.New("invalid public pricing")
	}
	if len(raw) > maxPricingBytes {
		return nil, false, errors.New("invalid public pricing")
	}
	p, err := parsePublicTariff(raw)
	if err == nil {
		p.observed = time.Now().UTC()
	}
	return p, false, err
}

// Parse the JSON data block only; never execute provider JavaScript. A missing
// price is distinct from zero, and an ambiguous data block fails closed.
func parsePublicTariff(raw []byte) (*publicTariff, error) {
	invalid := errors.New("invalid public pricing")
	if len(raw) > maxPricingBytes {
		return nil, invalid
	}
	z := html.NewTokenizer(bytes.NewReader(raw))
	var p *publicTariff
	var ipv4 *float64
	inScript, inIPs := false, false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if z.Err() != io.EOF {
				return nil, invalid
			}
			break
		}
		switch tt {
		case html.StartTagToken:
			t := z.Token()
			if t.Data == "script" {
				inScript = false
				for _, a := range t.Attr {
					if a.Key == "id" && a.Val == "calculation-js-js-extra" {
						inScript = true
					}
				}
			}
			if t.Data == "select" {
				inIPs = false
				for _, a := range t.Attr {
					if a.Key == "id" && a.Val == "NumberOfIPs" {
						inIPs = true
					}
				}
			}
			if t.Data == "option" && inIPs {
				value, price := "", ""
				for _, a := range t.Attr {
					if a.Key == "value" {
						value = a.Val
					}
					if a.Key == "data-price" {
						price = a.Val
					}
				}
				if value == "1" {
					n, e := strconv.ParseFloat(price, 64)
					if e != nil || !validAmount(n, false) || ipv4 != nil {
						return nil, invalid
					}
					ipv4 = &n
				}
			}
		case html.EndTagToken:
			t := z.Token()
			if t.Data == "script" {
				inScript = false
			}
			if t.Data == "select" {
				inIPs = false
			}
		case html.TextToken:
			if !inScript {
				continue
			}
			s := strings.TrimSpace(string(z.Text()))
			assignment := ""
			for _, prefix := range []string{"var KAMATERA_DATA", "const KAMATERA_DATA", "window.KAMATERA_DATA"} {
				if strings.HasPrefix(s, prefix) {
					assignment = prefix
					break
				}
			}
			if assignment == "" {
				continue
			}
			if p != nil {
				return nil, invalid
			}
			s = strings.TrimSpace(strings.TrimPrefix(s, assignment))
			if !strings.HasPrefix(s, "=") {
				return nil, invalid
			}
			s = strings.TrimSpace(strings.TrimPrefix(s, "="))
			dec := json.NewDecoder(strings.NewReader(s))
			var v publicTariff
			if dec.Decode(&v) != nil || !uniqueJSONKeys([]byte(s[:dec.InputOffset()])) {
				return nil, invalid
			}
			rest := strings.TrimSpace(s[dec.InputOffset():])
			rest = strings.TrimSpace(strings.TrimSuffix(rest, "//# sourceURL=calculation-js-js-extra"))
			if rest != "" && rest != ";" {
				return nil, invalid
			}
			p = &v
		}
	}
	if p == nil || ipv4 == nil || len(p.CPU) == 0 || len(p.CPU) > 256 || len(p.RAM) == 0 || len(p.RAM) > 8 || len(p.Disk) == 0 || len(p.Disk) > 512 || len(p.Regions) == 0 || len(p.Regions) > 128 || len(p.Images) == 0 || len(p.Images) > 4096 {
		return nil, invalid
	}
	p.ipv4 = *ipv4
	return p, nil
}
func validAmount(v float64, negative bool) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v <= 1000000 && ((negative && v >= -1000000) || v >= 0)
}
func componentPrice(xs []tariffOption, value string, negative bool) (float64, bool) {
	var price float64
	matches := 0
	for _, x := range xs {
		if x.Value != value {
			continue
		}
		if x.Price == nil || !validAmount(*x.Price, negative) {
			return 0, false
		}
		price = *x.Price
		matches++
	}
	return price, matches == 1
}
func hasString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func (p *publicTariff) quote(s sizeData, region string, images []imageData) (providers.PlanPrice, bool) {
	return p.quoteWithNativeLicences(s, region, images, nil)
}

func (p *publicTariff) quoteWithNativeLicences(s sizeData, region string, images []imageData, native map[string]bool) (providers.PlanPrice, bool) {
	if p == nil || len(p.Regions[region]) == 0 {
		return providers.PlanPrice{}, false
	}
	cpu, a := componentPrice(p.CPU, fmt.Sprintf("%d%s", s.Cores, s.CPUType), false)
	ram, b := componentPrice(p.RAM[s.CPUType], strconv.Itoa(s.RAM), true)
	disk, c := componentPrice(p.Disk, strconv.Itoa(s.Disk), false)
	if !a || !b || !c {
		return providers.PlanPrice{}, false
	}
	// Every selectable Ubuntu variant needs verified zero licence cost. Native
	// regional evidence may fill a missing public mapping, never a missing price
	// or a conflicting/ambiguous public image identity.
	matched := false
	for _, im := range images {
		if !hasString([]string{"22.04", "24.04", "26.04"}, im.version()) {
			continue
		}
		matches, identityMatches := 0, 0
		for _, os := range p.Images {
			if os.Name == strings.TrimPrefix(im.Code, region+":") || os.ID == strings.TrimPrefix(im.ID, region+":") {
				identityMatches++
				if os.Price == nil || *os.Price != 0 {
					return providers.PlanPrice{}, false
				}
				if hasString(os.Regions, region) || (native[im.ID] && strings.HasPrefix(im.ID, region+":")) {
					matches++
				}
			}
		}
		if matches != 1 || identityMatches != 1 {
			return providers.PlanPrice{}, false
		}
		matched = true
	}
	monthly := math.Round((cpu+ram+disk+p.ipv4)*1e6) / 1e6
	if !matched || !validAmount(monthly, false) || monthly <= 0 {
		return providers.PlanPrice{}, false
	}
	components := []providers.PriceComponent{}
	for _, v := range []struct {
		name, code string
		amount     float64
	}{{"CPU", fmt.Sprintf("%d%s", s.Cores, s.CPUType), cpu}, {"RAM", strconv.Itoa(s.RAM), ram}, {"SSD", strconv.Itoa(s.Disk), disk}, {"IPv4", "1", p.ipv4}} {
		components = append(components, providers.PriceComponent{Name: v.name, PlanCode: v.code, Quantity: 1, Hourly: v.amount / 730})
	}
	return providers.PlanPrice{Currency: "USD", Hourly: monthly / 730, MonthlyEstimate: monthly, Complete: true, Scope: publicPriceScope, Components: components}, true
}

// JSON duplicate keys would otherwise silently replace a price or region.
func uniqueJSONKeys(raw []byte) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	var value func(int) bool
	value = func(depth int) bool {
		if depth > 32 {
			return false
		}
		t, e := d.Token()
		if e != nil {
			return false
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return true
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				t, e = d.Token()
				if e != nil {
					return false
				}
				k, ok := t.(string)
				if !ok || seen[k] {
					return false
				}
				seen[k] = true
				if !value(depth + 1) {
					return false
				}
			}
			t, e = d.Token()
			return e == nil && t == json.Delim('}')
		case '[':
			for d.More() {
				if !value(depth + 1) {
					return false
				}
			}
			t, e = d.Token()
			return e == nil && t == json.Delim(']')
		}
		return false
	}
	return value(0)
}
