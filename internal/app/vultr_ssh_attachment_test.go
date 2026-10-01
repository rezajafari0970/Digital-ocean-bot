package app

import "testing"

func TestSSHKeyAttachment(t *testing.T) {
	tests := []struct {
		expected        string
		actual          []string
		attached, known bool
	}{
		{"", []string{"k1"}, false, false},
		{"k1", []string{"k1"}, true, true},
		{"k1", []string{"k2"}, false, true},
		{"k1", nil, false, true},
	}
	for _, tt := range tests {
		got, known := sshKeyAttachment(tt.expected, tt.actual)
		if got != tt.attached || known != tt.known {
			t.Fatalf("expected=%q actual=%v got=(%v,%v) want=(%v,%v)", tt.expected, tt.actual, got, known, tt.attached, tt.known)
		}
	}
}
