package app

import "testing"

func TestGenericDoesNotRewrite(t *testing.T) {
	g := proxyAdapterByName("generic")
	if g.Name() != "generic" {
		t.Fatal(g.Name())
	}
	if got := proxySessionUsername("generic", "plain-user", "de", "abc", true); got != "plain-user" {
		t.Fatal(got)
	}
}
func TestSuffixAdapterPreservesPolicy(t *testing.T) {
	a := proxyAdapterByName("suffix-session")
	if !a.Capabilities().StickySession {
		t.Fatal("no sticky")
	}
	got := proxySessionUsername("suffix-session", "user__sid.old", "de", "abc", true)
	if got != "user__cr.de;sid.abc" {
		t.Fatal(got)
	}
}
