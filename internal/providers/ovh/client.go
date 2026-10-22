// Package ovh implements project-scoped OVH Public Cloud accounts.
package ovh

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
)

var endpoints = map[string]string{"ovh-eu": "https://eu.api.ovh.com/1.0", "ovh-ca": "https://ca.api.ovh.com/1.0", "ovh-us": "https://api.us.ovhcloud.com/1.0"}
var projectPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var uuidPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var regionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,63}$`)

type credential struct {
	Endpoint          string `json:"endpoint"`
	ApplicationKey    string `json:"application_key"`
	ApplicationSecret string `json:"application_secret"`
	ConsumerKey       string `json:"consumer_key"`
	Project           string `json:"project_id"`
}

func problem(class providers.ErrorClass, op, code string, status int) *providers.Error {
	return cloudutil.Problem("OVH", class, op, code, status)
}
func signingCredentials(ctx context.Context, s providers.CredentialSource) (credential, error) {
	var c credential
	if cloudutil.ReadCredential(ctx, s, &c) != nil || endpoints[c.Endpoint] == "" || c.ApplicationKey == "" || c.ApplicationSecret == "" || c.ConsumerKey == "" || strings.ContainsAny(c.ApplicationKey+c.ApplicationSecret+c.ConsumerKey, "\r\n\x00") {
		return c, problem(providers.ErrorAuthentication, "credential", "INVALID_CREDENTIAL_JSON", 0)
	}
	return c, nil
}

func credentials(ctx context.Context, s providers.CredentialSource) (credential, error) {
	c, e := signingCredentials(ctx, s)
	if e != nil {
		return c, e
	}
	if !projectPattern.MatchString(c.Project) {
		return c, problem(providers.ErrorInvalidRequest, "credential", "PROJECT_SELECTION_REQUIRED", 0)
	}
	return c, nil
}

type client struct {
	http              *http.Client
	credentials       providers.CredentialSource
	endpoint, project string
	mu                sync.Mutex
	offset            time.Duration
	synced            time.Time
}

func (c *client) clock(ctx context.Context) (time.Time, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.synced) < 5*time.Minute {
		return time.Now().Add(c.offset), nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoints[c.endpoint]+"/auth/time", nil)
	start := time.Now()
	r, e := c.http.Do(req)
	if e != nil {
		return time.Time{}, problem(providers.ErrorTransport, "clock", "CLOCK_UNAVAILABLE", 0)
	}
	defer r.Body.Close()
	b, e := io.ReadAll(io.LimitReader(r.Body, 128))
	if e != nil || r.StatusCode != 200 {
		return time.Time{}, problem(providers.ErrorUnavailable, "clock", "CLOCK_UNAVAILABLE", r.StatusCode)
	}
	n, e := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if e != nil || n <= 0 {
		return time.Time{}, problem(providers.ErrorUnavailable, "clock", "INVALID_CLOCK", 200)
	}
	offset := time.Unix(n, 0).Sub(start.Add(time.Since(start) / 2))
	if offset < -24*time.Hour || offset > 24*time.Hour {
		return time.Time{}, problem(providers.ErrorUnavailable, "clock", "CLOCK_SKEW_EXCESSIVE", 200)
	}
	c.offset = offset
	c.synced = time.Now()
	return time.Now().Add(offset), nil
}
func signature(secret, consumer, method, url, body, stamp string) string {
	sum := sha1.Sum([]byte(strings.Join([]string{secret, consumer, method, url, body, stamp}, "+")))
	return "$1$" + hex.EncodeToString(sum[:])
}
func (c *client) do(ctx context.Context, method, path string, body, out any) error {
	cred, e := signingCredentials(ctx, c.credentials)
	if e != nil {
		return e
	}
	if cred.Project != c.project || cred.Endpoint != c.endpoint {
		return problem(providers.ErrorAuthentication, "credential", "PROJECT_SCOPE_CHANGED", 0)
	}
	b := []byte{}
	if body != nil {
		b, e = json.Marshal(body)
		if e != nil {
			return problem(providers.ErrorInvalidRequest, path, "INVALID_REQUEST", 0)
		}
	}
	now, e := c.clock(ctx)
	if e != nil {
		return e
	}
	stamp := strconv.FormatInt(now.Unix(), 10)
	endpoint := endpoints[c.endpoint] + path
	req, e := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(b))
	if e != nil {
		return problem(providers.ErrorInvalidRequest, path, "INVALID_REQUEST", 0)
	}
	req.Header.Set("X-Ovh-Application", cred.ApplicationKey)
	req.Header.Set("X-Ovh-Consumer", cred.ConsumerKey)
	req.Header.Set("X-Ovh-Timestamp", stamp)
	req.Header.Set("X-Ovh-Signature", signature(cred.ApplicationSecret, cred.ConsumerKey, method, endpoint, string(b), stamp))
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	r, e := c.http.Do(req)
	if e != nil {
		return problem(providers.ErrorTransport, path, "TRANSPORT_ERROR", 0)
	}
	defer r.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(r.Body, (8<<20)+1))
	if e != nil || len(raw) > 8<<20 {
		return problem(providers.ErrorUnavailable, path, "INVALID_RESPONSE_BODY", r.StatusCode)
	}
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return classify(path, r.StatusCode, raw)
	}
	if out != nil && (bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, out) != nil) {
		return problem(providers.ErrorUnavailable, path, "INVALID_RESPONSE", r.StatusCode)
	}
	return nil
}
func (c *client) path(suffix string) string { return "/cloud/project/" + c.project + suffix }
func classify(op string, status int, raw []byte) *providers.Error {
	class, label := providers.ErrorUnknown, "REQUEST_FAILED"
	switch {
	case status == 401:
		class, label = providers.ErrorAuthentication, "AUTHENTICATION_FAILED"
	case status == 402:
		class, label = providers.ErrorBilling, "BILLING_BLOCKED"
	case status == 403:
		class, label = providers.ErrorPermissionDenied, "PERMISSION_DENIED"
	case status == 404:
		class, label = providers.ErrorNotFound, "NOT_FOUND"
	case status == 429:
		class, label = providers.ErrorRateLimited, "RATE_LIMITED"
	case status >= 500 || status >= 300 && status < 400:
		class, label = providers.ErrorUnavailable, "PROVIDER_UNAVAILABLE"
	case status >= 400 && status < 500:
		class, label = providers.ErrorInvalidRequest, "REQUEST_REJECTED"
	}
	var x struct {
		Code    string `json:"errorCode"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &x)
	if status == 400 || status == 409 {
		m := strings.ToLower(x.Message)
		if strings.Contains(m, "quota") {
			class, label = providers.ErrorCapacity, "COMPUTE_QUOTA_EXCEEDED"
		} else if strings.Contains(m, "capacity") || strings.Contains(m, "out of stock") {
			class, label = providers.ErrorRegionCapacity, "REGION_CAPACITY_UNAVAILABLE"
		}
	}
	return problem(class, op, label, status)
}
func list[T any](ctx context.Context, c *client, path string, id func(T) string) ([]T, error) {
	var out []T
	if e := c.do(ctx, http.MethodGet, path, nil, &out); e != nil {
		return nil, e
	}
	if out == nil {
		return nil, problem(providers.ErrorUnavailable, path, "MISSING_COLLECTION", 200)
	}
	seen := map[string]bool{}
	for _, x := range out {
		k := id(x)
		if k == "" || seen[k] {
			return nil, problem(providers.ErrorUnavailable, path, "INVALID_COLLECTION_IDENTITY", 200)
		}
		seen[k] = true
	}
	return out, nil
}
