package vultr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type catalogAuditTransport func(*http.Request) (*http.Response, error)

func (f catalogAuditTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

var catalogAuditReaders = []struct {
	field string
	read  func(context.Context, *Client) (int, error)
}{
	{"regions", func(ctx context.Context, c *Client) (int, error) { xs, e := c.Regions(ctx); return len(xs), e }},
	{"plans", func(ctx context.Context, c *Client) (int, error) { xs, e := c.Plans(ctx); return len(xs), e }},
	{"os", func(ctx context.Context, c *Client) (int, error) { xs, e := c.OS(ctx); return len(xs), e }},
}

func catalogAuditClient(fn func(*http.Request) (int, string)) *Client {
	return NewClient(&http.Client{Transport: catalogAuditTransport(func(r *http.Request) (*http.Response, error) {
		status, body := fn(r)
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}, "fixture")
}

func catalogAuditPage(field, next string) string {
	b, _ := json.Marshal(map[string]any{
		field:  []map[string]any{{}},
		"meta": map[string]any{"links": map[string]string{"next": next}},
	})
	return string(b)
}

func TestCatalogPaginationUsesNativeCursors(t *testing.T) {
	for _, reader := range catalogAuditReaders {
		for _, form := range []string{"native", "absolute", "relative"} {
			t.Run(reader.field+"/"+form, func(t *testing.T) {
				const cursor = "opaque/cursor+="
				calls := 0
				c := catalogAuditClient(func(r *http.Request) (int, string) {
					calls++
					if r.URL.Host != "api.vultr.com" || r.URL.Path != "/v2/"+reader.field {
						t.Errorf("pagination changed endpoint: %s", r.URL.Redacted())
						return 400, "{}"
					}
					if r.URL.Query().Get("per_page") != "500" || len(r.URL.Query()) > 2 {
						t.Errorf("pagination query not bounded: %s", r.URL.RawQuery)
						return 400, "{}"
					}
					if calls == 1 {
						next := cursor
						if form == "absolute" {
							next = "https://untrusted.invalid/v2/other?cursor=opaque%2Fcursor%2B%3D&per_page=999999&unexpected=value"
						} else if form == "relative" {
							next = "/v2/" + reader.field + "?cursor=opaque%2Fcursor%2B%3D"
						}
						return 200, catalogAuditPage(reader.field, next)
					}
					if calls != 2 || r.URL.Query().Get("cursor") != cursor {
						t.Errorf("cursor corrupted or replayed: calls=%d cursor=%q", calls, r.URL.Query().Get("cursor"))
						return 400, "{}"
					}
					return 200, catalogAuditPage(reader.field, "")
				})
				n, err := reader.read(context.Background(), c)
				if err != nil || n != 2 || calls != 2 {
					t.Fatalf("items=%d requests=%d error=%v", n, calls, err)
				}
			})
		}
	}
}

func TestCatalogPaginationRejectsInvalidInventory(t *testing.T) {
	for _, reader := range catalogAuditReaders {
		for _, body := range []string{"{}", `{"%s":null}`, `{"%s":[],"meta":{"links":{"next":"https://invalid.test/path"}}}`, `{"%s":[],"meta":{"links":{"next":"/v2/path?cursor="}}}`, `{"%s":[],"meta":{"links":{"next":"/v2/path?cursor=a&cursor=b"}}}`, `{"%s":[],"meta":{"links":{"next":"http://%%invalid?cursor=a"}}}`} {
			t.Run(reader.field+"/"+body, func(t *testing.T) {
				calls := 0
				c := catalogAuditClient(func(r *http.Request) (int, string) {
					calls++
					if calls > 1 {
						return 400, "{}"
					}
					if body == "{}" {
						return 200, body
					}
					return 200, fmt.Sprintf(body, reader.field)
				})
				n, err := reader.read(context.Background(), c)
				if err == nil || n != 0 || calls != 1 {
					t.Fatalf("incomplete inventory accepted or followed: items=%d calls=%d error=%v", n, calls, err)
				}
			})
		}
		t.Run(reader.field+"/empty-valid", func(t *testing.T) {
			c := catalogAuditClient(func(*http.Request) (int, string) { return 200, fmt.Sprintf(`{"%s":[]}`, reader.field) })
			if n, err := reader.read(context.Background(), c); n != 0 || err != nil {
				t.Fatal(n, err)
			}
		})
	}
}

func TestCatalogPaginationStopsCyclesAndLimits(t *testing.T) {
	for _, reader := range catalogAuditReaders {
		for _, cycle := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/cycle=%v", reader.field, cycle), func(t *testing.T) {
				calls := 0
				c := catalogAuditClient(func(*http.Request) (int, string) {
					calls++
					if calls > 1000 {
						return 400, "{}"
					}
					next := fmt.Sprintf("page-%d", calls)
					if cycle {
						next = "same"
					}
					return 200, catalogAuditPage(reader.field, next)
				})
				n, err := reader.read(context.Background(), c)
				want := 1000
				if cycle {
					want = 2
				}
				if err == nil || n != 0 || calls != want || !strings.Contains(err.Error(), "pagination") {
					t.Fatalf("items=%d requests=%d want=%d error=%v", n, calls, want, err)
				}
			})
		}
	}
}

func TestCatalogPaginationCancellation(t *testing.T) {
	for _, reader := range catalogAuditReaders {
		t.Run(reader.field, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			c := catalogAuditClient(func(*http.Request) (int, string) {
				calls++
				cancel()
				if calls > 1 {
					return 400, "{}"
				}
				return 200, catalogAuditPage(reader.field, "next")
			})
			n, err := reader.read(ctx, c)
			if n != 0 || calls != 1 || !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled list continued: items=%d calls=%d error=%v", n, calls, err)
			}
		})
	}
}
