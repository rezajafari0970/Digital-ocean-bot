package ipwire

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type exchangeRT func(*http.Request) (*http.Response, error)

func (f exchangeRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type badBody struct{}

func (badBody) Read([]byte) (int, error) { return 0, errors.New("credential must not leak") }
func (badBody) Close() error             { return nil }
func exchangePrepared(t *testing.T) Prepared {
	t.Helper()
	p, e := Prepare(Spec{Provider: "digitalocean", Family: 4, Product: "reserved", Action: "ALLOCATE", Region: "nyc3", ServerID: "123"})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestExchangeNoReplayRedirectAndBoundedBody(t *testing.T) {
	p := exchangePrepared(t)
	for _, status := range []int{202, 307, 308, 401, 429, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			rt := exchangeRT(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.GetBody != nil || r.Header.Get("Idempotency-Key") != "" || r.Method != "POST" || r.URL.String() != p.URL() {
					t.Fatal("replay or scope changed")
				}
				if r.Header.Get("Authorization") != "Bearer fixture" {
					t.Fatal("missing bound credential")
				}
				if d, ok := r.Context().Deadline(); !ok || time.Until(d) > 30*time.Second {
					t.Fatal("unbounded")
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != string(p.Body()) {
					t.Fatal("body changed")
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://other.invalid"}, "Retry-After": []string{"120"}}, Body: io.NopCloser(strings.NewReader(`{"reserved_ip":{"ip":"8.8.4.4","region":{"slug":"nyc3"},"droplet":null}}`))}, nil
			})
			reply := Exchange(context.Background(), p, rt, []byte("fixture"))
			result := p.DecodeReply(reply, time.Now())
			if calls != 1 {
				t.Fatal(calls)
			}
			if status == 202 && result.Outcome != "ACKNOWLEDGED" {
				t.Fatal(result)
			}
			if status != 202 && result.Outcome != "UNKNOWN" {
				t.Fatal(result)
			}
			if reply.Header.Get("Location") != "" {
				t.Fatal("retained unnecessary header")
			}
		})
	}
	for _, body := range []io.ReadCloser{io.NopCloser(strings.NewReader(strings.Repeat("x", (1<<20)+5))), badBody{}} {
		reply := Exchange(context.Background(), p, exchangeRT(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"180"}}, Body: body}, nil
		}), []byte("fixture"))
		result := p.DecodeReply(reply, time.Now())
		if reply.Complete || result.Outcome != "UNKNOWN" || result.RetryAt == nil {
			t.Fatal("partial/oversize/cooldown", result)
		}
	}
	if r := Exchange(context.Background(), p, nil, []byte("fixture")); r.Complete {
		t.Fatal("default transport used")
	}
}
func TestExchangeActualTransportDroppedConnectionAndRedirect(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		t.Run(map[bool]string{false: "drop", true: "redirect"}[redirect], func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if redirect {
					w.Header().Set("Location", "https://must-not-follow.invalid/")
					w.WriteHeader(307)
					return
				}
				conn, _, e := w.(http.Hijacker).Hijack()
				if e != nil {
					t.Error(e)
					return
				}
				conn.Close()
			}))
			defer srv.Close()
			transport := srv.Client().Transport.(*http.Transport).Clone()
			defer transport.CloseIdleConnections()
			transport.Proxy = nil
			transport.TLSClientConfig.ServerName = srv.Certificate().DNSNames[0]
			transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
			}
			p := exchangePrepared(t)
			reply := Exchange(context.Background(), p, transport, []byte("fixture"))
			result := p.DecodeReply(reply, time.Now())
			if calls.Load() != 1 || result.Outcome != "UNKNOWN" {
				t.Fatal(calls.Load(), result)
			}
		})
	}
}
func TestDecodeReplyIncompleteNeverAcquires(t *testing.T) {
	p := exchangePrepared(t)
	r := p.DecodeReply(Reply{Status: 202, Body: []byte(`{"reserved_ip":{"ip":"8.8.4.4","region":{"slug":"nyc3"},"droplet":null}}`)}, time.Now())
	if r.Outcome != "UNKNOWN" || r.ResourceID != "" || len(r.Candidates) != 1 {
		t.Fatal(r)
	}
}
