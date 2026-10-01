package provisioning

import (
	"errors"
	"strings"
	"testing"
)

func TestStepRetryLimitErrorPreservesRootDiagnostic(t *testing.T) {
	err := stepRetryLimitError("installer-sanaei", 3, "ssh command failed: Process exited with status 1")
	if !errors.Is(err, ErrStepRetryLimit) {
		t.Fatalf("must wrap retry limit: %v", err)
	}
	for _, want := range []string{"step=installer-sanaei", "attempts=3", "status 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in %q", want, err.Error())
		}
	}
}
