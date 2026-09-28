package adminapi

import "testing"

func TestConfigExpressions(t *testing.T) {
	q, e := parseQuotaMB("1024")
	if e != nil || q != 1073741824 {
		t.Fatalf("quota %d %v", q, e)
	}
	q, e = parseQuotaMB("10 * 100")
	if e != nil || q != 1048576000 {
		t.Fatalf("product quota %d %v", q, e)
	}
	life, e := parseLifetimeMinutes("5*60")
	if e != nil || life != 18000 {
		t.Fatalf("life %d %v", life, e)
	}
	z, e := parseLifetimeMinutes("0")
	if e != nil || z != 0 {
		t.Fatalf("zero %d %v", z, e)
	}
}
