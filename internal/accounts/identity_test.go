package accounts

import "testing"

func TestRuntimeIdentitiesAreIndependent(t *testing.T) {
	a, err := NewRuntimeIdentity()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewRuntimeIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if a.CellID == b.CellID || a.RequestScope == b.RequestScope || a.CorrelationScope == b.CorrelationScope || a.AuditScope == b.AuditScope {
		t.Fatal("runtime identities must be independent")
	}
	if a.CellID == a.RequestScope || a.RequestScope == a.CorrelationScope {
		t.Fatal("identity purposes must not reuse values")
	}
}
