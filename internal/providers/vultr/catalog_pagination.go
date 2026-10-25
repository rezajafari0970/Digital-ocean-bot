package vultr

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

// Catalog pagination stays on the selected endpoint. Native next values are
// opaque cursors; legacy URL-shaped values contribute only their cursor.
type catalogPager struct {
	endpoint string
	next     string
	seen     map[string]bool
}

func newCatalogPager(endpoint string) *catalogPager {
	return &catalogPager{endpoint: endpoint, next: pagePath(endpoint, 500), seen: make(map[string]bool)}
}

func (p *catalogPager) path(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if p.seen[p.next] || len(p.seen) >= 1000 {
		return "", errors.New("vultr catalog pagination cycle or limit")
	}
	p.seen[p.next] = true
	return p.next, nil
}

func (p *catalogPager) advance(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		p.next = ""
		return nil
	}
	if len(raw) > 8192 {
		return errors.New("vultr catalog pagination cursor too large")
	}
	cursor := raw
	if strings.Contains(raw, "?") || strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "" && u.Scheme != "https" && u.Scheme != "http") || u.Fragment != "" || u.User != nil {
			return errors.New("vultr catalog pagination link invalid")
		}
		q, err := url.ParseQuery(u.RawQuery)
		if err != nil || len(q["cursor"]) != 1 || q.Get("cursor") == "" {
			return errors.New("vultr catalog pagination cursor missing or ambiguous")
		}
		cursor = q.Get("cursor")
	}
	p.next = p.endpoint + "?" + url.Values{"per_page": {"500"}, "cursor": {cursor}}.Encode()
	return nil
}
