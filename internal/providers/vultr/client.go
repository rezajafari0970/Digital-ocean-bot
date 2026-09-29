package vultr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

const defaultBaseURL = "https://api.vultr.com/v2"

type HTTPError struct {
	Status     int
	Message    string
	RetryAfter time.Duration
}

func (e HTTPError) Error() string { return fmt.Sprintf("vultr http %d: %s", e.Status, e.Message) }

type Client struct {
	http        *http.Client
	token       string
	credentials providers.CredentialSource
	base        string
}

func NewClient(h *http.Client, token string) *Client {
	return &Client{http: h, token: strings.TrimSpace(token), base: defaultBaseURL}
}
func NewClientFromSource(h *http.Client, source providers.CredentialSource) *Client {
	return &Client{http: h, credentials: source, base: defaultBaseURL}
}
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	if method != http.MethodGet {
		return c.doOnce(ctx, method, path, body, out)
	}
	for attempt := 0; attempt < 3; attempt++ {
		err := c.doOnce(ctx, method, path, body, out)
		if err == nil {
			return nil
		}
		var h HTTPError
		if !errors.As(err, &h) || h.Status != 429 || attempt == 2 {
			return err
		}
		d := h.RetryAfter
		if d <= 0 {
			d = time.Duration(250*(1<<attempt)) * time.Millisecond
		}
		if d > 5*time.Second {
			d = 5 * time.Second
		}
		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	return nil
}

func (c *Client) doOnce(ctx context.Context, method, path string, body any, out any) error {
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
	token := c.token
	if c.credentials != nil {
		b, getErr := c.credentials.Get(ctx)
		if getErr != nil {
			return getErr
		}
		defer zeroCredential(b)
		token = strings.TrimSpace(string(b))
	}
	if token == "" {
		return errors.New("vultr: empty credential")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var x struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &x)
		if x.Error == "" {
			x.Error = strings.TrimSpace(string(b))
		}
		var retry time.Duration
		if v := strings.TrimSpace(resp.Header.Get("Retry-After")); v != "" {
			if n, e := strconv.Atoi(v); e == nil && n >= 0 {
				retry = time.Duration(n) * time.Second
			}
		}
		return HTTPError{Status: resp.StatusCode, Message: x.Error, RetryAfter: retry}
	}
	if out != nil && len(b) > 0 {
		return json.Unmarshal(b, out)
	}
	return nil
}
func pagePath(path string, perPage int) string {
	u, _ := url.Parse(path)
	q := u.Query()
	q.Set("per_page", fmt.Sprint(perPage))
	u.RawQuery = q.Encode()
	return u.String()
}

func zeroCredential(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
