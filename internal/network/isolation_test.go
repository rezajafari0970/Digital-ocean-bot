package network

import "testing"

func TestAccountContextsCannotCross(t *testing.T) {
	a := &AccountContext{AccountID: "account-a", Profile: Profile{AccountID: "account-a"}}
	if err := a.Validate("account-b"); err == nil {
		t.Fatal("cross-account context must be rejected")
	}
}

func TestAccountNamespaceStableAndDistinct(t *testing.T) {
	a1 := AccountNamespace("account-a")
	a2 := AccountNamespace("account-a")
	b := AccountNamespace("account-b")
	if a1 != a2 {
		t.Fatal("namespace must be stable")
	}
	if a1 == b {
		t.Fatal("accounts must have distinct namespaces")
	}
}
