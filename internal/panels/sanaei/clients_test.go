package sanaei

import (
	"regexp"
	"testing"
)

func TestUUIDv4(t *testing.T) {
	seen := map[string]bool{}
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for i := 0; i < 100; i++ {
		id, err := UUIDv4()
		if err != nil {
			t.Fatal(err)
		}
		if !re.MatchString(id) {
			t.Fatalf("invalid uuid %s", id)
		}
		if seen[id] {
			t.Fatal("duplicate uuid")
		}
		seen[id] = true
	}
}
