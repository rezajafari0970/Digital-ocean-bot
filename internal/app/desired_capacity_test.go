package app

import "testing"

func TestDesiredServerHardCeiling(t *testing.T) {
	tests := []struct {
		name                        string
		desired, managed, preCreate int
		want                        bool
	}{
		{"empty account below desired", 5, 0, 0, true},
		{"one slot remains", 5, 4, 0, true},
		{"precreate consumes last slot", 5, 4, 1, false},
		{"managed equals desired", 5, 5, 0, false},
		{"managed exceeds desired", 5, 6, 0, false},
		{"multiple pending reach desired", 5, 2, 3, false},
		{"zero desired never creates", 0, 0, 0, false},
		{"negative inputs fail closed", 5, -1, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := desiredAllowsCreate(tt.desired, tt.managed, tt.preCreate); got != tt.want {
				t.Fatalf("desiredAllowsCreate(%d,%d,%d)=%v want %v", tt.desired, tt.managed, tt.preCreate, got, tt.want)
			}
		})
	}
}
