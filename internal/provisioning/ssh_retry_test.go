package provisioning

import (
	"testing"
	"time"
)

func TestSSHRetryWindowIsBoundedWithoutChangingInstallerBackoff(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want time.Duration
	}{{1, 15 * time.Second}, {2, 30 * time.Second}, {3, time.Minute}, {30, time.Minute}} {
		if got := stepRetryDelay("ssh", tc.n); got != tc.want {
			t.Fatalf("attempt %d: %v", tc.n, got)
		}
	}
	if stepRetryDelay("bootstrap", 6) != 64*time.Minute {
		t.Fatal("installer backoff changed")
	}
}
