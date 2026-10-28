// Package kamatera uses Kamatera's published CloudCLI REST contract.
// https://github.com/Kamatera/libcloud-driver-kamatera
package kamatera

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const endpoint = "https://cloudcli.cloudwm.com"

var uuidPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var regionPattern = regexp.MustCompile(`^[A-Z][A-Z0-9-]{0,23}$`)

type credential struct {
	ClientID string `json:"client_id"`
	Secret   string `json:"secret"`
}

func credentials(ctx context.Context, s providers.CredentialSource) (credential, error) {
	var c credential
	if err := ctx.Err(); err != nil {
		return c, err
	}
	if s == nil {
		return c, problem(providers.ErrorAuthentication, "credential", "INVALID_CREDENTIAL", 0)
	}
	if err := cloudutil.ReadCredential(ctx, s, &c); err != nil {
		// A canceled database/secret read does not reject the API credential.
		if ctx.Err() != nil {
			return c, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return c, context.DeadlineExceeded
		}
		if errors.Is(err, context.Canceled) {
			return c, context.Canceled
		}
		return c, problem(providers.ErrorAuthentication, "credential", "INVALID_CREDENTIAL", 0)
	}
	c.ClientID = strings.TrimSpace(c.ClientID)
	c.Secret = strings.TrimSpace(c.Secret)
	if c.ClientID == "" || c.Secret == "" || strings.IndexFunc(c.ClientID+c.Secret, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return c, problem(providers.ErrorAuthentication, "credential", "INVALID_CREDENTIAL", 0)
	}
	return c, nil
}
func CredentialScope(id string) string { return fingerprint(strings.TrimSpace(id)) }
func fingerprint(id string) string {
	h := sha256.Sum256([]byte(id))
	return "kamatera-key:" + hex.EncodeToString(h[:])
}
func problem(class providers.ErrorClass, op, code string, status int) *providers.Error {
	return cloudutil.Problem("Kamatera", class, op, code, status)
}

type client struct {
	http        *http.Client
	credentials providers.CredentialSource
	expected    string
}

func (c *client) do(ctx context.Context, method, path string, body, out any) (result error) {
	defer func() {
		var pe *providers.Error
		if !errors.As(result, &pe) {
			return
		}
		op := ""
		switch {
		case strings.Contains(path, "capabilities=1"):
			op = "capabilities"
		case strings.Contains(path, "sizes=1"):
			op = "plans"
		case strings.Contains(path, "images=1"):
			op = "images"
		case path == "/service/server?datacenter=1":
			op = "regions"
		}
		if op != "" {
			copy := *pe
			copy.Operation = op
			result = &copy
		}
	}()
	err := c.doOnce(ctx, method, path, body, out)
	if method != "GET" || ctx.Err() != nil {
		return err
	}
	var pe *providers.Error
	// Retry only transient reads once, inside the caller's original deadline.
	// Authentication, malformed JSON and all POST operations are not replayed.
	if !errors.As(err, &pe) || !(pe.Class == providers.ErrorTransport || pe.Code == "PROVIDER_UNAVAILABLE") {
		return err
	}
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	return c.doOnce(ctx, method, path, body, out)
}
func (c *client) doOnce(ctx context.Context, method, path string, body, out any) error {
	return c.doOnceAt(ctx, endpoint, method, path, body, out)
}

// apiBase is supplied only by the driver's pinned CloudCLI/native read callers.
func (c *client) doOnceAt(ctx context.Context, apiBase, method, path string, body, out any) error {
	cred, e := credentials(ctx, c.credentials)
	if e != nil {
		return e
	}
	if c.expected != "" && c.expected != fingerprint(cred.ClientID) {
		return problem(providers.ErrorAuthentication, "account", "CREDENTIAL_SCOPE_CHANGED", 0)
	}
	var raw []byte
	if body != nil {
		raw, e = json.Marshal(body)
		if e != nil {
			return problem(providers.ErrorInvalidRequest, "request", "INVALID_REQUEST", 0)
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, apiBase+path, bytes.NewReader(raw))
	if e != nil {
		return problem(providers.ErrorInvalidRequest, "request", "INVALID_REQUEST", 0)
	}
	req.Header.Set("AuthClientId", cred.ClientID)
	req.Header.Set("AuthSecret", cred.Secret)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, e := c.http.Do(req)
	if e != nil {
		return problem(providers.ErrorTransport, "request", "TRANSPORT_ERROR", 0)
	}
	defer resp.Body.Close()
	raw, e = io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if e != nil || len(raw) > 8<<20 {
		return problem(providers.ErrorUnavailable, "response", "INVALID_RESPONSE", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The deployed CloudCLI returns this exact 500 for an empty wildcard
		// read, despite allowNoServers in its public schema. This is only a
		// selection hint: rawServers must corroborate it with the basic list.
		if resp.StatusCode == http.StatusInternalServerError && method == http.MethodPost && path == "/service/server/info" {
			selector, ok := body.(map[string]string)
			var response map[string]json.RawMessage
			var message string
			if ok && len(selector) == 1 && selector["name"] == ".*" &&
				json.Unmarshal(raw, &response) == nil && len(response) == 1 &&
				json.Unmarshal(response["message"], &message) == nil && message == "No servers found (name='.*')" {
				return problem(providers.ErrorUnavailable, "inventory", "EMPTY_WILDCARD_SELECTION", resp.StatusCode)
			}
		}
		class, code := providers.ErrorUnknown, "REQUEST_FAILED"
		switch resp.StatusCode {
		case 401:
			class, code = providers.ErrorAuthentication, "CREDENTIAL_REJECTED"
		case 403:
			class, code = providers.ErrorPermissionDenied, "ACCESS_DENIED"
		case 429:
			class, code = providers.ErrorRateLimited, "RATE_LIMITED"
		case 400, 422:
			class, code = providers.ErrorInvalidRequest, "REQUEST_REJECTED"
		default:
			if resp.StatusCode >= 500 {
				class, code = providers.ErrorUnavailable, "PROVIDER_UNAVAILABLE"
			}
		}
		// A generic 404 or malformed body is never proof that a server disappeared.
		return problem(class, "request", code, resp.StatusCode)
	}
	if ((apiBase == "https://console.kamatera.com" && method == http.MethodGet) || (method == http.MethodGet && path == "/service/servers") || (method == http.MethodPost && path == "/service/server/info")) && resp.StatusCode != http.StatusOK {
		return problem(providers.ErrorUnavailable, "inventory", "INVALID_INVENTORY_STATUS", resp.StatusCode)
	}
	if out != nil && (bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, out) != nil) {
		return problem(providers.ErrorUnavailable, "response", "INVALID_RESPONSE", resp.StatusCode)
	}
	return nil
}

// CloudCLI represents an exact read as POST. Only its documented read payloads
// may pass the controller's mutation fence; body readers are not consumed.
func ReadOnlyRequest(r *http.Request) bool {
	if r.Method != "POST" || r.URL.Scheme != "https" || r.URL.Host != "cloudcli.cloudwm.com" || r.URL.Path != "/service/server/info" || r.URL.RawQuery != "" || r.GetBody == nil {
		return false
	}
	b, e := r.GetBody()
	if e != nil {
		return false
	}
	defer b.Close()
	raw, e := io.ReadAll(io.LimitReader(b, 513))
	if e != nil || len(raw) > 512 {
		return false
	}
	var m map[string]string
	if json.Unmarshal(raw, &m) != nil || len(m) != 1 {
		return false
	}
	return m["name"] == ".*" || uuidPattern.MatchString(m["id"])
}
func TerminationTarget(r *http.Request) string {
	if r.Method != "POST" || r.URL.Scheme != "https" || r.URL.Host != "cloudcli.cloudwm.com" || r.URL.Path != "/service/server/terminate" || r.URL.RawQuery != "" || r.GetBody == nil {
		return ""
	}
	b, e := r.GetBody()
	if e != nil {
		return ""
	}
	defer b.Close()
	raw, e := io.ReadAll(io.LimitReader(b, 513))
	if e != nil || len(raw) > 512 {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil || len(m) != 2 {
		return ""
	}
	var id string
	var force bool
	if json.Unmarshal(m["id"], &id) != nil || json.Unmarshal(m["force"], &force) != nil || !force || !uuidPattern.MatchString(id) {
		return ""
	}
	return id
}
