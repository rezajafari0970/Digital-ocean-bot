package vultr

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSSHDeletionInventoryRejectsMissingIdentity(t *testing.T) {
	for _, scenario := range []string{"first-page", "later-page", "null-entry", "valid-empty", "valid-paginated"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/ssh-keys" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if scenario == "valid-empty" {
					fmt.Fprint(w, `{"ssh_keys":[],"meta":{"links":{"next":""}}}`)
					return
				}
				if calls == 1 && (scenario == "later-page" || scenario == "valid-paginated") {
					fmt.Fprint(w, `{"ssh_keys":[{"id":"owned","name":"managed"}],"meta":{"links":{"next":"next"}}}`)
					return
				}
				if scenario == "valid-paginated" {
					fmt.Fprint(w, `{"ssh_keys":[{"id":"foreign","name":"manual"}],"meta":{"links":{"next":""}}}`)
					return
				}
				if scenario == "null-entry" {
					fmt.Fprint(w, `{"ssh_keys":[null],"meta":{"links":{"next":""}}}`)
					return
				}
				fmt.Fprint(w, `{"ssh_keys":[{"name":"managed"}],"meta":{"links":{"next":""}}}`)
			}))
			defer srv.Close()
			client := NewClient(srv.Client(), "fixture")
			client.base = srv.URL
			keys, err := (&Driver{client: client}).ListSSHKeys(context.Background())
			if scenario == "valid-empty" || scenario == "valid-paginated" {
				want := 0
				if scenario == "valid-paginated" {
					want = 2
				}
				if err != nil || len(keys) != want {
					t.Fatalf("valid inventory: keys=%d err=%v", len(keys), err)
				}
			} else if err == nil || keys != nil {
				t.Fatalf("invalid inventory became absence evidence: keys=%d err=%v", len(keys), err)
			}
			wantCalls := 1
			if scenario == "later-page" || scenario == "valid-paginated" {
				wantCalls = 2
			}
			if calls != wantCalls {
				t.Fatalf("unexpected pagination calls=%d want=%d", calls, wantCalls)
			}
		})
	}
}
