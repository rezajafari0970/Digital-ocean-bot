package rollingreboot

import "testing"

func TestMax(t *testing.T) {
	if max(1, 2) != 2 || max(3, 2) != 3 {
		t.Fatal("max")
	}
}
