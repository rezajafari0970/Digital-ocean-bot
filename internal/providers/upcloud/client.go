package upcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const apiBase = "https://api.upcloud.com/1.3"
const maxBody = 8 << 20

var safeCode = regexp.MustCompile("^[A-Z0-9_-]{1,96}$")

type Client struct {
	http   *http.Client
	source providers.CredentialSource
	base   string
}
type apiError struct {
	status int
	code   string
	retry  time.Duration
}

func (e apiError) Error() string { return fmt.Sprintf("UpCloud HTTP %d (%s)", e.status, e.code) }

// Preserve the account gateway and mutation fence; never follow redirects.
func newClient(h *http.Client, source providers.CredentialSource) *Client {
	copy := *h
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if copy.Timeout == 0 || copy.Timeout > 30*time.Second {
		copy.Timeout = 30 * time.Second
	}
	return &Client{http: &copy, source: source, base: apiBase}
}
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	secret, err := c.source.Get(ctx)
	if err != nil {
		return err
	}
	defer func() {
		for i := range secret {
			secret[i] = 0
		}
	}()
	token := strings.TrimSpace(string(secret))
	if token == "" {
		return &providers.Error{Class: providers.ErrorAuthentication, Message: "UpCloud API token is empty"}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil {
		return &responseError{Code: "RESPONSE_READ_FAILED", Status: res.StatusCode}
	}
	if len(b) > maxBody {
		return &responseError{Code: "RESPONSE_TOO_LARGE", Status: res.StatusCode}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// Raw provider/proxy error bodies can echo secrets; persist only safe codes.
		var e struct {
			Error struct {
				Code string `json:"error_code"`
			} `json:"error"`
			Code string `json:"error_code"`
		}
		_ = json.Unmarshal(b, &e)
		code := e.Error.Code
		if code == "" {
			code = e.Code
		}
		if !safeCode.MatchString(code) {
			code = "REQUEST_FAILED"
		}
		var retry time.Duration
		if n, e := strconv.Atoi(res.Header.Get("Retry-After")); e == nil && n >= 0 {
			retry = time.Duration(min(n, 3600)) * time.Second
		} else if at, e := http.ParseTime(res.Header.Get("Retry-After")); e == nil {
			retry = max(0, min(time.Hour, time.Until(at)))
		}
		return apiError{res.StatusCode, code, retry}
	}
	if out != nil {
		if len(bytes.TrimSpace(b)) == 0 {
			return &responseError{Code: "EMPTY_RESPONSE", Status: res.StatusCode}
		}
		if err := json.Unmarshal(b, out); err != nil {
			var re *responseError
			if errors.As(err, &re) {
				return &responseError{Code: re.Code, Status: res.StatusCode}
			}
			return &responseError{Code: "INVALID_JSON", Status: res.StatusCode}
		}
	}
	return nil
}
func normalize(op string, err error) error {
	if err == nil {
		return nil
	}
	var p *providers.Error
	if errors.As(err, &p) {
		return err
	}
	class := providers.ErrorTransport
	var response *responseError
	if errors.As(err, &response) {
		return withDiagnostic(&providers.Error{Class: providers.ErrorUnavailable, Operation: op, StatusCode: response.Status, Message: "UpCloud returned an unusable response", Cause: err}, response.Code)
	}
	var e apiError
	if errors.As(err, &e) {
		switch {
		case e.status == 401:
			class = providers.ErrorAuthentication
		case e.status == 402:
			class = providers.ErrorBilling
		case e.code == "ACCOUNT_LOCKED" || e.code == "ACCOUNT_SUSPENDED":
			class = providers.ErrorAccountLocked
		case strings.HasSuffix(e.code, "_LIMIT_REACHED") && (strings.HasPrefix(e.code, "SERVER_") || strings.Contains(e.code, "STORAGE") || e.code == "PUBLIC_IPV4_LIMIT_REACHED"):
			class = providers.ErrorCapacity
		case e.code == "SERVER_RESOURCES_UNAVAILABLE" || e.code == "STORAGE_RESOURCES_UNAVAILABLE" || e.code == "IP_ADDRESS_RESOURCES_UNAVAILABLE":
			class = providers.ErrorRegionCapacity
		case e.status == 403:
			class = providers.ErrorPermissionDenied
		case e.status == 404:
			class = providers.ErrorNotFound
		case e.status == 429:
			class = providers.ErrorRateLimited
		case e.code == "SERVER_STATE_ILLEGAL" || e.code == "STORAGE_STATE_ILLEGAL" || e.code == "STORAGE_ATTACHED" || e.status >= 500 || e.status == 408:
			class = providers.ErrorUnavailable
		default:
			class = providers.ErrorInvalidRequest
		}
	}
	message := "UpCloud request failed through account transport"
	if e.status != 0 {
		message = e.Error()
	}
	result := &providers.Error{Class: class, Operation: op, StatusCode: e.status, RetryAfter: e.retry, Message: message, Cause: err}
	if e.status != 0 {
		return withDiagnostic(result, apiDiagnostic(e.status, e.code))
	}
	return result
}

// UpCloud encodes numeric fields as either JSON strings or integers.
type number int64

func (n *number) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), "\"")
	v, e := strconv.ParseInt(s, 10, 64)
	if e != nil || v < 0 || v > 1<<50 {
		return &responseError{Code: "INVALID_NUMBER"}
	}
	*n = number(v)
	return nil
}

type label struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func labelValue(xs []label, key string) string {
	for _, x := range xs {
		if x.Key == key {
			return x.Value
		}
	}
	return ""
}

type accountData struct {
	Username string            `json:"username"`
	Credits  json.Number       `json:"credits"`
	Limits   map[string]number `json:"resource_limits"`
}
type planData struct {
	Name   string `json:"name"`
	CPU    number `json:"core_number"`
	Memory number `json:"memory_amount"`
	Disk   number `json:"storage_size"`
	Tier   string `json:"storage_tier"`
	GPU    number `json:"gpu_amount"`
}
type ipData struct {
	Address string `json:"address"`
	Family  string `json:"family"`
	Access  string `json:"access"`
	Server  string `json:"server"`
}
type serverData struct {
	ID       string          `json:"uuid"`
	Title    string          `json:"title"`
	Hostname string          `json:"hostname"`
	State    string          `json:"state"`
	Zone     string          `json:"zone"`
	Plan     string          `json:"plan"`
	CPU      number          `json:"core_number"`
	Memory   number          `json:"memory_amount"`
	Created  json.RawMessage `json:"created"`
	Labels   struct {
		Items []label `json:"label"`
	} `json:"labels"`
	Tags struct {
		Items []string `json:"tag"`
	} `json:"tags"`
	IPs struct {
		Items []ipData `json:"ip_address"`
	} `json:"ip_addresses"`
	Disks struct {
		Items []struct {
			ID   string `json:"storage"`
			Type string `json:"type"`
		} `json:"storage_device"`
	} `json:"storage_devices"`
}
type storageData struct {
	ID           string  `json:"uuid"`
	Title        string  `json:"title"`
	Type         string  `json:"type"`
	Access       string  `json:"access"`
	State        string  `json:"state"`
	Tier         string  `json:"tier"`
	Zone         string  `json:"zone"`
	TemplateType string  `json:"template_type"`
	Size         number  `json:"size"`
	Labels       []label `json:"labels"`
	Servers      struct {
		Items []string `json:"server"`
	} `json:"servers"`
}
