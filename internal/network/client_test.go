package network

import "testing"

func TestClientsAreNotSharedAcrossAccounts(t *testing.T) {
	a, err := NewIsolatedDirectClient("a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewIsolatedDirectClient("b")
	if err != nil {
		t.Fatal(err)
	}
	if a.Client == b.Client {
		t.Fatal("http client shared")
	}
	if a.Jar == b.Jar {
		t.Fatal("cookie jar shared")
	}
	if a.Transport == b.Transport {
		t.Fatal("transport shared")
	}
	if err := a.Validate("b"); err == nil {
		t.Fatal("cross-account client use must fail")
	}
}
