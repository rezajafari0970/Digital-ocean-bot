package healthverify

import (
	"strings"
	"testing"
)

func TestHealthFailureThreshold(t *testing.T) {
	if stateAfterFailure(1) != "DEGRADED" || stateAfterFailure(2) != "DEGRADED" || stateAfterFailure(3) != "UNHEALTHY" {
		t.Fatal("unexpected health failure threshold")
	}
}

func TestHealthCommandUsesBoundSocksPort(t *testing.T) {
	cmd := command("cfg", "203.0.113.10", 443, 39081)
	if !strings.Contains(cmd, "SOCKS_PORT=39081") || strings.Contains(cmd, "SOCKS_PORT=%!") {
		t.Fatalf("bad socks port command: %s", cmd)
	}
}
