package advancedreality

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func evidenceFixture(now time.Time) Evidence {
	h := strings.Repeat("a", 64)
	v := Evidence{Schema: 1, StartedAt: now.Add(-time.Minute), FinishedAt: now,
		Scope: "stock_sidecar", ClientVersion: "26.5.9", ClientCoreSHA256: h, ServerCoreSHA256: h,
		ConfigSHA256: h, TargetSHA256: h, NetworkASN: 197207, RouteMatched: true, Direct: true,
		ClientUnchanged: true, ControlPassed: true}
	for _, n := range []string{"a", "b", "c"} {
		v.Tests = append(v.Tests, TransferEvidence{Nonce: strings.Repeat(n, 32), At: now, Stage: "payload",
			UploadBytes: 16384, DownloadBytes: 65536, UploadVerified: true, DownloadVerified: true, Success: true})
	}
	return v
}
func assessFixture(v Evidence, now time.Time) (EvidenceAssessment, error) {
	b, _ := json.Marshal(v)
	return AssessConnectivity(b, now)
}
func TestConnectivityProofBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 14, 2, 30, 0, 0, time.UTC)
	tests := []struct {
		name   string
		change func(*Evidence)
		status string
		err    error
	}{
		{"sidecar_never_grants_rollout", func(v *Evidence) {}, "BIDIRECTIONAL_SAMPLE", nil},
		{"tiny_transfer_is_not_bulk", func(v *Evidence) {
			for i := range v.Tests {
				v.Tests[i].UploadBytes = 128
				v.Tests[i].DownloadBytes = 226
			}
		}, "PARTIAL_TRANSFER", nil},
		{"partial_does_not_pass", func(v *Evidence) {
			v.Tests[1].Success = false
			v.Tests[1].UploadVerified = false
			v.Tests[1].DownloadVerified = false
		}, "PARTIAL_TRANSFER", nil},
		{"tls_is_not_data", func(v *Evidence) {
			for i := range v.Tests {
				v.Tests[i].Stage = "tls"
				v.Tests[i].Success = false
			}
		}, "NO_VERIFIED_TRANSFER", nil},
		{"echo_service_not_target_route", func(v *Evidence) { v.RouteMatched = false }, "UNVERIFIED_TEST_CONTEXT", nil},
		{"broken_control", func(v *Evidence) { v.ControlPassed = false }, "UNVERIFIED_TEST_CONTEXT", nil},
		{"relay", func(v *Evidence) { v.Direct = false }, "UNVERIFIED_TEST_CONTEXT", nil},
		{"changed_client", func(v *Evidence) { v.ClientUnchanged = false }, "UNVERIFIED_TEST_CONTEXT", nil},
		{"missing_asn", func(v *Evidence) { v.NetworkASN = 0 }, "UNVERIFIED_TEST_CONTEXT", nil},
		{"future", func(v *Evidence) { v.FinishedAt = now.Add(time.Second) }, "", ErrEvidence},
		{"stale", func(v *Evidence) { v.StartedAt = now.Add(-12 * time.Minute); v.FinishedAt = now.Add(-11 * time.Minute) }, "", ErrEvidence},
		{"replayed_nonce", func(v *Evidence) { v.Tests[1].Nonce = v.Tests[0].Nonce }, "", ErrEvidence},
		{"hash_missing", func(v *Evidence) { v.ClientCoreSHA256 = "" }, "", ErrEvidence},
		{"version_unknown", func(v *Evidence) { v.ClientVersion = "26.10.10" }, "", ErrVersion},
		{"false_success", func(v *Evidence) { v.Tests[0].DownloadVerified = false }, "", ErrEvidence},
		{"handshake_only_success", func(v *Evidence) { v.Tests[0].Stage = "tls" }, "", ErrEvidence},
		{"zero_payload", func(v *Evidence) { v.Tests[0].UploadBytes = 0 }, "", ErrEvidence},
		{"future_trial", func(v *Evidence) { v.Tests[0].At = now.Add(time.Second) }, "", ErrEvidence},
		{"empty", func(v *Evidence) { v.Tests = nil }, "", ErrEvidence},
		{"raw_tls_cannot_prove_proxy", func(v *Evidence) { v.Scope = "raw_tls" }, "BIDIRECTIONAL_SAMPLE", nil},
		{"app_requires_soak", func(v *Evidence) { v.Scope = "v2rayng_tun" }, "BIDIRECTIONAL_SAMPLE", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := evidenceFixture(now)
			tt.change(&v)
			a, e := assessFixture(v, now)
			if !errors.Is(e, tt.err) {
				t.Fatalf("error %v want %v", e, tt.err)
			}
			if e == nil && (a.Status != tt.status || a.CanApply || len(a.Blockers) < 2) {
				t.Fatalf("unsafe assessment: %+v", a)
			}
		})
	}
}
func TestEvidenceInputDoesNotLeak(t *testing.T) {
	now := time.Now()
	for _, raw := range []string{`{"schema":1,"schema":1,"privateKey":"DO_NOT_ECHO"}`, `{"privateKey":"DO_NOT_ECHO"}`, strings.Repeat("x", 65537)} {
		_, e := AssessConnectivity([]byte(raw), now)
		if e != ErrEvidence || strings.Contains(e.Error(), "DO_NOT_ECHO") {
			t.Fatal("ambiguous or sensitive input accepted")
		}
	}
	v := evidenceFixture(now)
	b, _ := json.Marshal(v)
	a, e := AssessConnectivity(b, now)
	if e != nil {
		t.Fatal(e)
	}
	out, _ := json.Marshal(a)
	if strings.Contains(string(out), v.TargetSHA256) || strings.Contains(string(out), v.Tests[0].Nonce) {
		t.Fatal("assessment must contain aggregate data only")
	}
}
