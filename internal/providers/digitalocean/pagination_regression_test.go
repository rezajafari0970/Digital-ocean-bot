package digitalocean

import (
	"encoding/json"
	"testing"
)

func TestExtractNextStripsAPIVersionPrefix(t *testing.T) {
	raw := json.RawMessage(`{"pages":{"next":"https://api.digitalocean.com/v2/sizes?page=2&per_page=20"}}`)
	if got := extractNext(raw); got != "sizes?page=2&per_page=20" {
		t.Fatalf("got %q", got)
	}
}
