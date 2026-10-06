package providers

import "testing"

func TestUpCloudTrialPortBoundaries(t *testing.T) {
	for _, port := range []int{-1, 0, 22, 53, 123, 2053, 3106, 3389, 4630, 10000, 65536} {
		if UpCloudTrialProxyPortAllowed(port) {
			t.Fatal("unsupported proxy service port", port)
		}
	}
	for _, port := range []int{80, 443, 8080} {
		if !UpCloudTrialProxyPortAllowed(port) {
			t.Fatal("documented proxy service port", port)
		}
	}
	if UpCloudTrialClientPortAllowed(3389) || UpCloudTrialClientPortAllowed(8080) || !UpCloudTrialClientPortAllowed(443) {
		t.Fatal("client/management collision")
	}
}
