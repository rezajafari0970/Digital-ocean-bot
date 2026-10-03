package adminapi

import (
	"os"
	"strings"
	"testing"
)

func TestProxyHealthHandlerDoesNotWriteAccountNetworkIdentity(t *testing.T) {
	b, err := os.ReadFile("proxy_health.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if strings.Contains(src, "INSERT INTO account_network_identities") {
		t.Fatal("manual proxy health test must not write account_network_identities")
	}
	if !strings.Contains(src, "Sticky Keeper is the sole writer") {
		t.Fatal("identity writer ownership invariant is undocumented")
	}
}
