package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type APIError struct {
	Operation  string
	Status     int
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	switch e.Status {
	case 401, 403:
		switch e.Operation {
		case "dns_read":
			return "cloudflare_dns_read_permission_denied"
		case "dns_write":
			return "cloudflare_dns_write_permission_denied"
		case "certificate":
			return "cloudflare_certificate_permission_denied"
		case "ssl":
			return "cloudflare_ssl_permission_denied"
		}
		return "cloudflare_permission_denied"
	case 429:
		return "cloudflare_rate_limited"
	default:
		return "cloudflare_api_unavailable"
	}
}

type Client struct {
	Auth   Auth
	HTTP   *http.Client
	base   string
	Before func(context.Context) error
}

func NewClient(a Auth) *Client {
	return &Client{Auth: a, base: "https://api.cloudflare.com/client/v4", HTTP: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type envelope struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Info    struct {
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

func (c *Client) request(ctx context.Context, method, path string, body any, out any) (int, error) {
	if e := c.Auth.Validate(); e != nil {
		return 0, e
	}
	if c.Before != nil {
		if e := c.Before(ctx); e != nil {
			return 0, e
		}
	}
	var data []byte
	var e error
	if body != nil {
		data, e = json.Marshal(body)
		if e != nil {
			return 0, e
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(data))
	if e != nil {
		return 0, errors.New("cloudflare_request_invalid")
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Auth.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Auth.Token)
	} else {
		req.Header.Set("X-Auth-Key", c.Auth.Key)
		req.Header.Set("X-Auth-Email", c.Auth.Email)
	}
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return 0, errors.New("cloudflare_transport_unconfirmed")
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if e != nil {
		return 0, errors.New("cloudflare_response_unconfirmed")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		delay := 60 * time.Second
		if n, e := strconv.Atoi(resp.Header.Get("Retry-After")); e == nil && n > 0 {
			delay = time.Duration(min(n, 3600)) * time.Second
		}
		return 0, &APIError{Status: resp.StatusCode, RetryAfter: delay, Operation: requestOperation(method, path)}
	}
	var env envelope
	if json.Unmarshal(raw, &env) != nil || !env.Success {
		return 0, errors.New("cloudflare_response_unconfirmed")
	}
	if out != nil && json.Unmarshal(env.Result, out) != nil {
		return 0, errors.New("cloudflare_response_invalid")
	}
	return env.Info.TotalPages, nil
}
func (c *Client) Zones(ctx context.Context) ([]Zone, error) {
	all := []Zone{}
	for page := 1; page <= 200; page++ {
		var batch []Zone
		pages, e := c.request(ctx, "GET", "/zones?per_page=50&page="+strconv.Itoa(page), nil, &batch)
		if e != nil {
			return nil, e
		}
		all = append(all, batch...)
		if page >= pages {
			return all, nil
		}
	}
	return nil, errors.New("cloudflare_zone_limit")
}
func (c *Client) Records(ctx context.Context, zone, name string) ([]Record, error) {
	var all []Record
	for page := 1; page <= 10; page++ {
		var batch []Record
		pages, e := c.request(ctx, "GET", "/zones/"+url.PathEscape(zone)+"/dns_records?name="+url.QueryEscape(name)+"&per_page=100&page="+strconv.Itoa(page), nil, &batch)
		if e != nil {
			return nil, e
		}
		all = append(all, batch...)
		if page >= pages {
			return all, nil
		}
	}
	return nil, errors.New("cloudflare_record_conflict")
}
func (c *Client) CreateRecord(ctx context.Context, z string, r Record) (Record, error) {
	var out Record
	_, e := c.request(ctx, "POST", "/zones/"+url.PathEscape(z)+"/dns_records", map[string]any{"type": r.Type, "name": r.Name, "content": r.Content, "comment": r.Comment, "proxied": r.Proxied, "ttl": r.TTL}, &out)
	return out, e
}
func (c *Client) DeleteRecord(ctx context.Context, z, id string) error {
	_, e := c.request(ctx, "DELETE", "/zones/"+url.PathEscape(z)+"/dns_records/"+url.PathEscape(id), nil, nil)
	return e
}
func (c *Client) Setting(ctx context.Context, z, key string) (string, error) {
	var out struct {
		Value string `json:"value"`
	}
	_, e := c.request(ctx, "GET", "/zones/"+url.PathEscape(z)+"/settings/"+url.PathEscape(key), nil, &out)
	return out.Value, e
}
func (c *Client) SetSetting(ctx context.Context, z, key, value string) error {
	_, e := c.request(ctx, "PATCH", "/zones/"+url.PathEscape(z)+"/settings/"+url.PathEscape(key), map[string]string{"value": value}, nil)
	return e
}
func (c *Client) Certificates(ctx context.Context, z string) ([]Certificate, error) {
	var out []Certificate
	_, e := c.request(ctx, "GET", "/certificates?zone_id="+url.QueryEscape(z), nil, &out)
	return out, e
}
func (c *Client) Issue(ctx context.Context, csr string, names []string) (Certificate, error) {
	var out Certificate
	_, e := c.request(ctx, "POST", "/certificates", map[string]any{"csr": csr, "hostnames": names, "request_type": "origin-rsa", "requested_validity": 7}, &out)
	return out, e
}

// Operation labels are a fixed allowlist; URLs, tokens and provider bodies are
// never included in persisted errors or API responses.
func requestOperation(method, path string) string {
	if strings.HasPrefix(path, "/certificates") {
		return "certificate"
	}
	if strings.Contains(path, "/dns_records") {
		if method == "GET" {
			return "dns_read"
		}
		return "dns_write"
	}
	if strings.Contains(path, "/settings/") {
		return "ssl"
	}
	return ""
}
