package kamatera

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// CloudCLI strips licensePrice from its image listing. Its official server
// implementation obtains that listing from this same native configuration GET.
// The fallback verifies only licences; public hardware prices remain mandatory.
func (d *Driver) nativeImageLicences(ctx context.Context, region string, images []imageData) map[string]bool {
	if !regionPattern.MatchString(region) {
		return nil
	}
	budget := 5 * time.Second
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline)-2*time.Second < budget {
		budget = time.Until(deadline) - 2*time.Second
	}
	if budget <= 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	httpClient := *d.client.http
	httpClient.Jar = nil
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c := *d.client
	c.http = &httpClient
	var raw json.RawMessage
	if c.doOnceAt(ctx, "https://console.kamatera.com", "GET", "/svc/serverCreate/datacenterConfiguration/"+region, nil, &raw) != nil {
		return nil
	}
	verified, err := parseNativeImageLicences(raw, region, images)
	if err != nil {
		return nil
	}
	return verified
}

func parseNativeImageLicences(raw []byte, region string, images []imageData) (map[string]bool, error) {
	invalid := errors.New("invalid regional image licence evidence")
	if len(raw) > 2<<20 || !uniqueJSONKeys(raw) {
		return nil, invalid
	}
	var config struct {
		Groups []struct {
			Images []struct {
				ID    string   `json:"id"`
				Price *float64 `json:"licensePrice"`
			} `json:"diskImages"`
		} `json:"serverCategoryDiskImages"`
	}
	if json.Unmarshal(raw, &config) != nil || len(config.Groups) == 0 || len(config.Groups) > 64 {
		return nil, invalid
	}
	wanted := map[string]bool{}
	for _, im := range images {
		if im.version() != "" && strings.HasPrefix(im.ID, region+":") {
			wanted[im.ID] = true
		}
	}
	seen, verified := map[string]bool{}, map[string]bool{}
	count := 0
	for _, group := range config.Groups {
		count += len(group.Images)
		if count > 4096 {
			return nil, invalid
		}
		for _, im := range group.Images {
			if !wanted[im.ID] {
				continue
			}
			if seen[im.ID] {
				return nil, invalid
			}
			seen[im.ID] = true
			if im.Price != nil && *im.Price == 0 {
				verified[im.ID] = true
			}
		}
	}
	return verified, nil
}

// Native evidence is consulted only for an exact, unique public image whose
// licence price is known to be zero but whose public regional mapping is absent.
func (p *publicTariff) needsNativeLicences(region string, images []imageData) bool {
	if p == nil || len(p.Regions[region]) == 0 {
		return false
	}
	for _, im := range images {
		if !hasString([]string{"22.04", "24.04", "26.04"}, im.version()) {
			continue
		}
		exact, regional := 0, 0
		for _, os := range p.Images {
			if os.ID == strings.TrimPrefix(im.ID, region+":") || os.Name == strings.TrimPrefix(im.Code, region+":") {
				if os.Price == nil || *os.Price != 0 {
					return false
				}
				exact++
				if hasString(os.Regions, region) {
					regional++
				}
			}
		}
		if exact == 1 && regional == 0 {
			return true
		}
	}
	return false
}
