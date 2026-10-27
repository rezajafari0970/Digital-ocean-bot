package capacity

import (
	"strings"
	"testing"
)

func TestCreateBlockReasonSeparatesMaintenanceFromProviderDenial(t *testing.T) {
	m := CreateBlockReason("KAMATERA_ASYNC_CREATE_REVIEW")
	if !strings.Contains(m, "local maintenance hold") || strings.Contains(m, "UpCloud") {
		t.Fatal(m)
	}
	if x := CreateBlockReason("UNRECOGNIZED_BLOCK"); strings.Contains(x, "UpCloud") {
		t.Fatal(x)
	}
	if x := CreateBlockReason("TRIAL_FIREWALL"); !strings.Contains(x, "UpCloud trial") {
		t.Fatal(x)
	}
}
