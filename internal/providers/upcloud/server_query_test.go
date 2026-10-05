package upcloud

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

// Contract source: https://developers.upcloud.com/api/1.3/upcloud-openapi.json
// listServersSortBy is the field; listServersOrderBy is the direction.
// The legacy prose server documentation reverses these parameter names.
func TestServerListCurrentAPIContract(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		total, pageSize, failOffset int
		offsets                     []int
	}{
		{"empty", 0, 100, -1, []int{0}},
		{"short_pages", 3, 2, -1, []int{0, 2, 3}},
		{"full_pages", 201, 100, -1, []int{0, 100, 200, 201}},
		{"first_page_400", 3, 2, 0, []int{0}},
		{"later_page_400", 3, 2, 2, []int{0, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var offsets []int
			d := fixture(t, func(r *http.Request) (int, any, error) {
				q := r.URL.Query()
				offset, err := strconv.Atoi(q.Get("offset"))
				// Reject the legacy inverted request just as the current API contract does.
				if r.Method != "GET" || r.URL.Path != "/1.3/server" || len(q) != 4 ||
					q.Get("sort_by") != "title" || q.Get("order_by") != "asc" ||
					q.Get("limit") != "100" || err != nil || offset < 0 {
					return http.StatusBadRequest, map[string]any{}, nil
				}
				offsets = append(offsets, offset)
				if offset == tc.failOffset {
					return http.StatusBadRequest, map[string]any{}, nil
				}
				xs := []serverData{}
				for i := offset; i < min(tc.total, offset+tc.pageSize); i++ {
					xs = append(xs, serverData{ID: fmt.Sprintf("00112233-4455-4677-8899-%012d", i)})
				}
				return http.StatusOK, map[string]any{"servers": map[string]any{"server": xs}}, nil
			}, nil)
			got, err := d.rawServers(context.Background())
			if tc.failOffset >= 0 {
				if err == nil || got != nil {
					t.Fatalf("400 returned partial inventory: %v, %v", got, err)
				}
				if providers.Class(err) != providers.ErrorInvalidRequest {
					t.Fatalf("classification: %v", err)
				}
			} else if err != nil || len(got) != tc.total {
				t.Fatalf("complete inventory: got %d, want %d: %v", len(got), tc.total, err)
			}
			if !reflect.DeepEqual(offsets, tc.offsets) {
				t.Fatalf("pagination/retries: got %v, want %v", offsets, tc.offsets)
			}
		})
	}
}
