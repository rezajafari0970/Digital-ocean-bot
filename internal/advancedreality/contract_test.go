package advancedreality

import (
	"errors"
	"strings"
	"testing"
)

const baseline = `{"policy":{"levels":{"0":{"bufferSize":64,"handshake":8}}},"inbounds":[{"protocol":"vless","settings":{"clients":[{"id":"secret-user"}]},"streamSettings":{"security":"reality","realitySettings":{"privateKey":"secret-key","serverNames":["example.test"]}}}],"outbounds":[{"protocol":"freedom"}]}`

func TestStructureAndSecurityContract(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		err       error
	}{
		{"unchanged", baseline, nil},
		{"bounded_buffer", strings.Replace(baseline, `"bufferSize":64`, `"bufferSize":128`, 1), nil},
		{"bounded_handshake", strings.Replace(baseline, `"handshake":8`, `"handshake":12`, 1), nil},
		{"new_key", strings.Replace(baseline, `"handshake":8`, `"handshake":8,"connIdle":10`, 1), ErrShape},
		{"removed_key", strings.Replace(baseline, `"bufferSize":64,`, "", 1), ErrShape},
		{"numeric_string", strings.Replace(baseline, `"bufferSize":64`, `"bufferSize":"64"`, 1), ErrShape},
		{"cipher_or_key", strings.Replace(baseline, "secret-key", "new-key", 1), ErrImmutable},
		{"published_sni", strings.Replace(baseline, "example.test", "other.test", 1), ErrImmutable},
		{"client_identity", strings.Replace(baseline, "secret-user", "new-user", 1), ErrImmutable},
		{"client_count", strings.Replace(baseline, `[{"id":"secret-user"}]`, `[]`, 1), ErrShape},
		{"new_outbound", strings.Replace(baseline, `[{"protocol":"freedom"}]`, `[{"protocol":"freedom"},{"protocol":"blackhole"}]`, 1), ErrShape},
		{"disable_buffer", strings.Replace(baseline, `"bufferSize":64`, `"bufferSize":0`, 1), ErrRange},
		{"unbounded_buffer", strings.Replace(baseline, `"bufferSize":64`, `"bufferSize":10000000`, 1), ErrRange},
		{"fractional", strings.Replace(baseline, `"bufferSize":64`, `"bufferSize":64.5`, 1), ErrRange},
		{"duplicate_key", strings.Replace(baseline, `"bufferSize":64`, `"bufferSize":64,"bufferSize":128`, 1), ErrJSON},
		{"escaped_duplicate", `{"a":1,"\u0061":2}`, ErrJSON},
		{"trailing_document", baseline + `{}`, ErrJSON},
		{"null_root", `null`, ErrJSON},
		{"oversize", `{"a":"` + strings.Repeat("a", MaxConfigBytes) + `"}`, ErrJSON},
		{"depth_limit", `{"a":` + strings.Repeat("[", 70) + `0` + strings.Repeat("]", 70) + `}`, ErrJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, e := Inspect([]byte(baseline), []byte(tt.candidate), "26.10.10")
			if !errors.Is(e, tt.err) {
				t.Fatalf("got %v want %v", e, tt.err)
			}
			if e == nil && (!p.StructurePreserved || p.CanApply || len(p.Blockers) == 0) {
				t.Fatal("unsafe plan")
			}
			if e != nil && (strings.Contains(e.Error(), "secret") || strings.Contains(e.Error(), "example.test")) {
				t.Fatal("sensitive error")
			}
		})
	}
}
func TestUnknownVersionAndArrayOrder(t *testing.T) {
	if _, e := Inspect([]byte(baseline), []byte(baseline), "26.10.11"); e != ErrVersion {
		t.Fatal(e)
	}
	a := []byte(`{"a":["first","second"]}`)
	b := []byte(`{"a":["second","first"]}`)
	if _, e := Inspect(a, b, "26.5.9"); e != ErrImmutable {
		t.Fatal(e)
	}
}
func TestUntrustedPathNotDisclosed(t *testing.T) {
	_, e := Inspect([]byte(`{"secret-key":1}`), []byte(`{"secret-key":2}`), "26.3.27")
	if e != ErrImmutable || strings.Contains(e.Error(), "secret") {
		t.Fatal(e)
	}
}
func FuzzInspectNeverAllowsShapeOrIdentityMutation(f *testing.F) {
	f.Add([]byte(baseline))
	f.Add([]byte(`{"a":1,"a":2}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		p, e := Inspect([]byte(baseline), b, "26.10.10")
		if e == nil {
			if p.CanApply || !p.StructurePreserved {
				t.Fatal("unsafe")
			}
			for _, path := range p.ChangedPaths {
				if !allowedNumeric(path) {
					t.Fatal("unapproved path")
				}
			}
		}
	})
}
