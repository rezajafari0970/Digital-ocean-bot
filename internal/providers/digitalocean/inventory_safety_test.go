package digitalocean

import (
	"context"
	"net/http"
	"testing"
)

func TestSSHInventoryRejectsNullAndBrokenPagination(t *testing.T) {
	for _, body := range []string{`{}`, `{"ssh_keys":null}`, `{"ssh_keys":[],"links":"invalid"}`, `{"ssh_keys":[],"links":{"pages":{"next":"https://api.digitalocean.com/v2/account/keys"}}}`} {
		c := fixtureClient(t, func(*http.Request) (int, string) { return 200, body })
		d, _ := NewDriver(c)
		if _, err := d.ListSSHKeys(context.Background()); err == nil {
			t.Fatal("ambiguous inventory accepted", body)
		}
	}
}
