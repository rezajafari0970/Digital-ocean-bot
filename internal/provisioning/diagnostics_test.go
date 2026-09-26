package provisioning

import (
	"errors"
	"testing"
)

func TestLinuxFailureTaxonomy(t *testing.T) {
	code := 100
	cases := []struct {
		name, stderr, want string
		retry              bool
	}{
		{"apt_lock", "E: Could not get lock /var/lib/dpkg/lock-frontend. It is held by process 123", "PKG_LOCK_BUSY", true},
		{"dns", "Temporary failure resolving 'archive.ubuntu.com'", "DNS_RESOLUTION_FAILED", true},
		{"disk", "write error: No space left on device", "DISK_FULL", false},
		{"permission", "/tmp/install.sh: Permission denied", "PERMISSION_DENIED", false},
		{"dpkg", "dpkg was interrupted, you must manually run dpkg --configure -a", "DPKG_INTERRUPTED", true},
		{"deps", "E: Unmet dependencies. Try apt --fix-broken install", "PKG_DEPENDENCY_FAILED", true},
		{"service", "Failed to start x.service: Unit x.service failed", "SERVICE_START_FAILED", true},
		{"disconnect", "client_loop: send disconnect: Broken pipe", "SSH_DISCONNECTED", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := ClassifyCommandFailure(errors.Join(ErrSSHCommand, errors.New("exit status 100")), CommandResult{Stderr: tc.stderr, ExitCode: &code})
			if d.Code != tc.want {
				t.Fatalf("code=%s want=%s", d.Code, tc.want)
			}
			if DiagnosticRetryable(d) != tc.retry {
				t.Fatalf("retry=%v", DiagnosticRetryable(d))
			}
			if d.Fingerprint == "" {
				t.Fatal("missing fingerprint")
			}
		})
	}
}
func TestUnknownFailureKeepsFingerprint(t *testing.T) {
	d := ClassifyCommandFailure(errors.Join(ErrSSHCommand, errors.New("mysterious future failure")), CommandResult{Stderr: "future daemon exploded code Z91"})
	if d.Fingerprint == "" {
		t.Fatal("missing fingerprint")
	}
	if d.Code != "SSH_COMMAND_FAILED" {
		t.Fatalf("code=%s", d.Code)
	}
}
func TestSSHFailureTaxonomy(t *testing.T) {
	cases := []struct{ msg, code string }{
		{"ssh not ready: ssh tcp dial: dial tcp 127.0.0.1:22: connect: connection refused", "SSH_CONNECTION_REFUSED"},
		{"ssh not ready: ssh handshake: ssh: handshake failed: ssh: unable to authenticate, attempted methods [none publickey], no supported methods remain", "SSH_AUTH_FAILED"},
		{"ssh not ready: ssh tcp dial: dial tcp: no route to host", "SSH_NO_ROUTE"},
		{"ssh not ready: ssh handshake: connection reset by peer", "SSH_HANDSHAKE_FAILED"},
	}
	for _, tc := range cases {
		d := ClassifyError(errors.Join(ErrSSHNotReady, errors.New(tc.msg)))
		if d.Code != tc.code {
			t.Fatalf("%q code=%s want=%s", tc.msg, d.Code, tc.code)
		}
	}
}
func TestFingerprintNormalizesDynamicValues(t *testing.T) {
	a := ClassifyError(errors.New("dial 10.1.2.3 process 12345 failed"))
	b := ClassifyError(errors.New("dial 192.168.9.8 process 98765 failed"))
	if a.Fingerprint != b.Fingerprint {
		t.Fatalf("dynamic values fragmented fingerprint: %s %s", a.Fingerprint, b.Fingerprint)
	}
}
