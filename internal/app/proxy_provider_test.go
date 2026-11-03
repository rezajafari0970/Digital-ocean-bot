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
	if got != "user__cr.de;sid.abc;sessttl.120" {
		t.Fatal(got)
	}
}

func TestEphemeralSessionDriverGrammar(t *testing.T) {
	for _, test := range []struct {
		adapter, base, country, session, want string
		targeted                              bool
	}{
		{"suffix-session", "user__sid.old;cr.fr", "it", "draft-one", "user__cr.it;sessid.draft-one;sessttl.120", true},
		{"suffix-session", "user__sessid.old", "", "draft-two", "user__sessid.draft-two;sessttl.120", false},
		{"fixed", "fixed-user", "it", "draft", "fixed-user", true},
		{"generic", "generic-user", "it", "draft", "generic-user", true},
	} {
		if got := proxyEphemeralSessionUsername(test.adapter, test.base, test.country, test.session, test.targeted); got != test.want {
			t.Fatalf("%s: got %q, want %q", test.adapter, got, test.want)
		}
	}
}
