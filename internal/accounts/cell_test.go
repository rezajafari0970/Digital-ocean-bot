package accounts

import "testing"

func TestCellManagerKeepsAccountsSeparate(t *testing.T) {
	m := NewCellManager()
	a := m.Register("a")
	b := m.Register("b")
	if a == b {
		t.Fatal("cells must be distinct")
	}
	if _, err := m.Acquire("missing"); err == nil {
		t.Fatal("missing cell must fail")
	}
	got, err := m.Acquire("a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Context.AccountID != "a" {
		t.Fatal("wrong tenant cell")
	}
}
