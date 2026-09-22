package accounts

import "testing"

func TestAccountNamespacesAreDistinct(t *testing.T) {
	a := NewContext("a")
	b := NewContext("b")
	if a.CacheNamespace == b.CacheNamespace {
		t.Fatal("cache namespace collision")
	}
	if a.SecretNamespace == b.SecretNamespace {
		t.Fatal("secret namespace collision")
	}
	if err := a.Authorize("b"); err == nil {
		t.Fatal("cross-account access must fail")
	}
}
