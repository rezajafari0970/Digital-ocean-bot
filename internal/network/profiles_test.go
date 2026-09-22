package network

import "testing"

func TestSecurityProfilesHaveSeparateState(t *testing.T) {
	ta := NewTLSProfile("a")
	tb := NewTLSProfile("b")
	if ta == tb || ta.Config == tb.Config || ta.SessionCache == tb.SessionCache {
		t.Fatal("TLS state shared")
	}

	ha, err := NewHTTPProfile("a")
	if err != nil {
		t.Fatal(err)
	}
	hb, err := NewHTTPProfile("b")
	if err != nil {
		t.Fatal(err)
	}
	if ha.Client == hb.Client || ha.Jar == hb.Jar || ha.Transport == hb.Transport {
		t.Fatal("HTTP state shared")
	}

	ba := NewBrowserProfile("a", "/var/lib/digital-ocean-bot/browser")
	bb := NewBrowserProfile("b", "/var/lib/digital-ocean-bot/browser")
	if ba.ProfileDir == bb.ProfileDir || ba.StorageNamespace == bb.StorageNamespace || ba.AuthNamespace == bb.AuthNamespace {
		t.Fatal("browser state shared")
	}
}
