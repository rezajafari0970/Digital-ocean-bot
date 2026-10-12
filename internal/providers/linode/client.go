package linode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

const endpoint = "https://api.linode.com/v4"
const maxBody = 8 << 20

type client struct {
	http        *http.Client
	credentials providers.CredentialSource
}

func newClient(h *http.Client, credentials providers.CredentialSource) *client {
	clone := *h // Preserve the application's proxy and mutation-fencing transport.
	clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if clone.Timeout <= 0 || clone.Timeout > 30*time.Second {
		clone.Timeout = 30 * time.Second
	}
	return &client{http: &clone, credentials: credentials}
}
func problem(class providers.ErrorClass, op, code string, status int) *providers.Error {
	return &providers.Error{Class: class, Operation: op, Code: code, StatusCode: status, Message: "Linode: " + code}
}
func (c *client) do(ctx context.Context, method, path string, body, out any) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return problem(providers.ErrorInvalidRequest, path, "INVALID_REQUEST", 0)
		}
	}
	token, err := c.credentials.Get(ctx)
	if err != nil {
		return problem(providers.ErrorAuthentication, path, "CREDENTIAL_UNAVAILABLE", 0)
	}
	defer func() {
		for i := range token {
			token[i] = 0
		}
	}()
	value := strings.TrimSpace(string(token))
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return problem(providers.ErrorAuthentication, path, "INVALID_CREDENTIAL", 0)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint+path, bytes.NewReader(data))
	if err != nil {
		return problem(providers.ErrorInvalidRequest, path, "INVALID_REQUEST", 0)
	}
	req.Header.Set("Authorization", "Bearer "+value)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return problem(providers.ErrorTransport, path, "TRANSPORT_ERROR", 0)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil {
		return problem(providers.ErrorUnavailable, path, "RESPONSE_READ_FAILED", res.StatusCode)
	}
	if len(raw) > maxBody {
		return problem(providers.ErrorUnavailable, path, "RESPONSE_TOO_LARGE", res.StatusCode)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		pe := classify(path, res.StatusCode, raw)
		pe.RetryAfter = retryAfter(res.Header)
		return pe
	}
	if out != nil && (bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, out) != nil) {
		return problem(providers.ErrorUnavailable, path, "INVALID_RESPONSE", res.StatusCode)
	}
	return nil
}

// Provider text is classified but never exposed in logs or diagnostics.
func classify(op string, status int, raw []byte) *providers.Error {
	class, code := providers.ErrorUnknown, "PROVIDER_ERROR"
	switch {
	case status == 401:
		class, code = providers.ErrorAuthentication, "AUTHENTICATION_FAILED"
	case status == 402:
		class, code = providers.ErrorBilling, "BILLING_BLOCKED"
	case status == 403:
		class, code = providers.ErrorPermissionDenied, "PERMISSION_DENIED"
	case status == 404:
		class, code = providers.ErrorNotFound, "NOT_FOUND"
	case status == 429:
		class, code = providers.ErrorRateLimited, "RATE_LIMITED"
	case status >= 500:
		class, code = providers.ErrorUnavailable, "PROVIDER_UNAVAILABLE"
	case status >= 300 && status < 400:
		class, code = providers.ErrorUnavailable, "REDIRECT_REJECTED"
	case status >= 400 && status < 500:
		class, code = providers.ErrorInvalidRequest, "REQUEST_REJECTED"
	}
	var payload struct {
		Errors []struct {
			Field  string `json:"field"`
			Reason string `json:"reason"`
		} `json:"errors"`
	}
	if (status == 400 || status == 403) && json.Unmarshal(raw, &payload) == nil {
		for _, item := range payload.Errors {
			reason := strings.ToLower(strings.TrimSpace(item.Reason))
			switch {
			case strings.HasPrefix(reason, "account limit reached"):
				class, code = providers.ErrorCapacity, "ACCOUNT_LIMIT_REACHED"
			case strings.Contains(reason, "insufficient capacity") || strings.Contains(reason, "not enough capacity"):
				class, code = providers.ErrorRegionCapacity, "REGION_CAPACITY_UNAVAILABLE"
			case item.Field == "image" && strings.Contains(reason, "not available"):
				class, code = providers.ErrorImageUnavailable, "IMAGE_UNAVAILABLE"
			}
		}
	}
	return problem(class, op, code, status)
}
func retryAfter(h http.Header) time.Duration {
	var delay time.Duration
	if n, err := strconv.ParseInt(h.Get("Retry-After"), 10, 64); err == nil && n > 0 {
		if n > 3600 {
			n = 3600
		}
		delay = time.Duration(n) * time.Second
	} else if at, err := http.ParseTime(h.Get("Retry-After")); err == nil {
		delay = time.Until(at)
	} else if n, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		delay = time.Until(time.Unix(n, 0))
	}
	if delay < 0 {
		return 0
	}
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

// Incomplete or inconsistent pagination must never erase known resources.
func list[T any](ctx context.Context, c *client, path string, identity func(T) string) ([]T, error) {
	out := []T{}
	seen := map[string]bool{}
	pages, total := 0, 0
	for page := 1; page <= 200; page++ {
		var envelope struct {
			Data    *[]T `json:"data"`
			Page    *int `json:"page"`
			Pages   *int `json:"pages"`
			Results *int `json:"results"`
		}
		if err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s?page=%d&page_size=500", path, page), nil, &envelope); err != nil {
			return nil, err
		}
		if envelope.Data == nil || envelope.Page == nil || envelope.Pages == nil || envelope.Results == nil ||
			*envelope.Page != page || *envelope.Pages < 1 || *envelope.Pages > 200 || *envelope.Results < 0 || *envelope.Results > 20000 {
			return nil, problem(providers.ErrorUnavailable, path, "INVALID_PAGINATION", 200)
		}
		if page == 1 {
			pages, total = *envelope.Pages, *envelope.Results
		}
		if *envelope.Pages != pages || *envelope.Results != total || (len(*envelope.Data) == 0 && total != 0) || len(*envelope.Data) > 500 {
			return nil, problem(providers.ErrorUnavailable, path, "INCOMPLETE_PAGINATION", 200)
		}
		for _, item := range *envelope.Data {
			id := identity(item)
			if id == "" || seen[id] {
				return nil, problem(providers.ErrorUnavailable, path, "INVALID_LIST_IDENTITY", 200)
			}
			seen[id] = true
			out = append(out, item)
		}
		if page == pages {
			if len(out) != total {
				return nil, problem(providers.ErrorUnavailable, path, "INCOMPLETE_PAGINATION", 200)
			}
			return out, nil
		}
	}
	return nil, problem(providers.ErrorUnavailable, path, "PAGINATION_BOUND_EXCEEDED", 200)
}
func isAmbiguous(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var pe *providers.Error
	return errors.As(err, &pe) && (pe.Class == providers.ErrorTransport || pe.Class == providers.ErrorUnavailable || pe.Class == providers.ErrorAmbiguousOutcome)
}
