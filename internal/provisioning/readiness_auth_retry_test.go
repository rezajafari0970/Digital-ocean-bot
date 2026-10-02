package provisioning

import "testing"

func TestReadinessSSHAuthIsRetryableDuringBootConvergence(t *testing.T) {
	e := &ReadinessError{Codes: []string{"SSH_AUTH_FAILED"}, Retryable: true}
	if !e.Retryable {
		t.Fatal("fresh-instance SSH auth failure must be retryable inside bounded readiness budget")
	}
	d := ClassifyError(errAuthForTest{})
	if d.Code != "SSH_AUTH_FAILED" {
		t.Fatalf("got %s", d.Code)
	}
}

type errAuthForTest struct{}

func (errAuthForTest) Error() string {
	return "ssh handshake: ssh: unable to authenticate, attempted methods [none publickey], no supported methods remain"
}
