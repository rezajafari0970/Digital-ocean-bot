package vultr

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeletionInventoriesFollowOpaqueCursors(t *testing.T) {
	for _, kind := range []string{"instances", "ssh-keys"} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.URL.Path != "/"+kind {
				t.Errorf("pagination left endpoint: %s", r.URL.Path)
			}
			field := "instances"
			if kind == "ssh-keys" {
				field = "ssh_keys"
			}
			if r.URL.Query().Get("cursor") == "" {
				fmt.Fprintf(w, `{"%s":[{"id":"first"}],"meta":{"links":{"next":"opaque/cursor+="}}}`, field)
			} else {
				if r.URL.Query().Get("cursor") != "opaque/cursor+=" {
					t.Error("cursor corrupted")
				}
				fmt.Fprintf(w, `{"%s":[{"id":"second"}],"meta":{"links":{"next":""}}}`, field)
			}
		}))
		c := NewClient(server.Client(), "fixture")
		c.base = server.URL
		if kind == "instances" {
			xs, err := c.Instances(context.Background())
			if err != nil || len(xs) != 2 {
				t.Fatal(xs, err)
			}
		} else {
			xs, err := (&Driver{client: c}).ListSSHKeys(context.Background())
			if err != nil || len(xs) != 2 {
				t.Fatal(xs, err)
			}
		}
		server.Close()
		if calls != 2 {
			t.Fatal(calls)
		}
	}
}
