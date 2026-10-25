package digitalocean

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/accounts"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

type auditedBody struct {
	reader io.Reader
	read   int
	closed bool
}

func (b *auditedBody) Read(p []byte) (int, error) { n, e := b.reader.Read(p); b.read += n; return n, e }
func (b *auditedBody) Close() error               { b.closed = true; return nil }

func responseAuditClient(t *testing.T, status int, body *auditedBody, calls *int) *Client {
	t.Helper()
	c, err := NewClient(accounts.NewContext("audit"), "token", secretStub{[]byte("fixture")}, &http.Client{
		Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
			*calls++
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: body, Request: r}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestProviderResponseReadBounds(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
			t.Run(method+"/"+http.StatusText(status), func(t *testing.T) {
				// Valid JSON much larger than either success or diagnostic allowance.
				body := &auditedBody{reader: io.MultiReader(strings.NewReader(`{"message":"`), strings.NewReader(strings.Repeat("x", 16<<20)), strings.NewReader(`"}`))}
				calls := 0
				c := responseAuditClient(t, status, body, &calls)
				var out map[string]any
				var err error
				if method == http.MethodGet {
					err = c.get(context.Background(), "/account", &out)
				} else {
					err = c.request(context.Background(), method, "/droplets", map[string]string{}, &out)
				}
				limit := (8 << 20) + 1
				if status != http.StatusOK {
					limit = 32 << 10
					var h HTTPError
					if !errors.As(err, &h) || h.Status != status {
						t.Fatalf("HTTP status lost: %v", err)
					}
				}
				if err == nil || calls != 1 || !body.closed || body.read > limit {
					t.Fatalf("unbounded/accepted response: bytes=%d limit=%d calls=%d closed=%v err=%T", body.read, limit, calls, body.closed, err)
				}
			})
		}
	}
}

func TestProviderResponseRequiresSingleJSONValue(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, raw := range []string{`{"account":{}} {"unexpected":true}`, `null`, ` `, `{"account":{}}garbage`} {
			t.Run(method+"/"+raw, func(t *testing.T) {
				body := &auditedBody{reader: strings.NewReader(raw)}
				calls := 0
				c := responseAuditClient(t, 200, body, &calls)
				var out map[string]any
				var err error
				if method == http.MethodGet {
					err = c.get(context.Background(), "/account", &out)
				} else {
					err = c.request(context.Background(), method, "/droplets", nil, &out)
				}
				if !errors.Is(err, ErrProviderRequest) || calls != 1 || !body.closed {
					t.Fatal(calls, body.closed, err)
				}
			})
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method+"/exact-boundary", func(t *testing.T) {
			raw := `{}` + strings.Repeat(" ", (8<<20)-2)
			body := &auditedBody{reader: strings.NewReader(raw)}
			calls := 0
			c := responseAuditClient(t, 200, body, &calls)
			var out map[string]any
			var err error
			if method == http.MethodGet {
				err = c.get(context.Background(), "/account", &out)
			} else {
				err = c.request(context.Background(), method, "/droplets", nil, &out)
			}
			if err != nil || out == nil || body.read != 8<<20 || !body.closed || calls != 1 {
				t.Fatal(body.read, body.closed, calls, err)
			}
		})
	}

}

func TestCreateResponseMissingIdentityRemainsAmbiguous(t *testing.T) {
	for _, kind := range []string{"droplet", "ssh_key"} {
		for _, raw := range []string{"", "{}", "null", `{"droplet":{"id":0},"ssh_key":{"id":0}}`, `{"droplet":{"id":-1},"ssh_key":{"id":-1}}`} {
			t.Run(kind+"/"+raw, func(t *testing.T) {
				body := &auditedBody{reader: strings.NewReader(raw)}
				calls := 0
				status := 200
				if raw == "" {
					status = 204
				}
				c := responseAuditClient(t, status, body, &calls)
				d, _ := NewDriver(c)
				if kind == "droplet" {
					result, err := d.CreateServer(context.Background(), providers.CreateServerRequest{Name: "audit"})
					if err == nil || result.Outcome != providers.OutcomeAmbiguous || result.ServerID != "" {
						t.Fatalf("result=%+v error=%v", result, err)
					}
				} else {
					result, err := d.CreateSSHKey(context.Background(), "audit", "fixture")
					if err == nil || result.ID != "" || !providers.IsRetryable(err) {
						t.Fatalf("result=%+v error=%v", result, err)
					}
				}
				if calls != 1 || !body.closed {
					t.Fatal(calls, body.closed)
				}
			})
		}
	}
}
