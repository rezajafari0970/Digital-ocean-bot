package provisioning

import "testing"

func issueByCode(xs []ReadinessIssue, code string) *ReadinessIssue {
	for i := range xs {
		if xs[i].Code == code {
			return &xs[i]
		}
	}
	return nil
}
func TestReadinessPolicyDecisions(t *testing.T) {
	s := ReadinessSnapshot{IsRoot: true, DiskFreeMB: 20000, PackageManager: "apt-get", PackageHealth: "ok", DNSOK: false, OutboundHTTPSOK: true, TimeSync: "yes", Checks: map[string]string{"package_lock": "free"}}
	xs := DecideReadiness(s)
	x := issueByCode(xs, "DNS_UNAVAILABLE")
	if x == nil || x.Action != "RETRY" {
		t.Fatalf("%+v", xs)
	}
	s.DNSOK = true
	s.DiskFreeMB = 500
	xs = DecideReadiness(s)
	x = issueByCode(xs, "DISK_LOW")
	if x == nil || x.Action != "BLOCK" {
		t.Fatalf("%+v", xs)
	}
	if readinessError(xs).Retryable {
		t.Fatal("disk low must be terminal")
	}
}
func TestPackageIncompleteUsesSafeRemediation(t *testing.T) {
	s := ReadinessSnapshot{IsRoot: true, DiskFreeMB: 20000, PackageManager: "apt-get", PackageHealth: "", DNSOK: true, OutboundHTTPSOK: true, TimeSync: "yes", Checks: map[string]string{"package_lock": "free"}}
	x := issueByCode(DecideReadiness(s), "PACKAGE_STATE_INCOMPLETE")
	if x == nil || x.Action != "AUTO_REMEDIATE" || x.Remediation != "dpkg --configure -a" {
		t.Fatalf("%+v", x)
	}
}
