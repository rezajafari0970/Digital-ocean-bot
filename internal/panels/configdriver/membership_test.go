package configdriver

import (
	"reflect"
	"testing"
)

func TestResizePreservesIndependentIdentities(t *testing.T) {
	old, e := NewCredentials("profile", "panel", []string{"DIRECT", "RESIDENTIAL"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	larger, e := Resize(old, "profile", "panel", []string{"DIRECT", "RESIDENTIAL"}, 3)
	if e != nil {
		t.Fatal(e)
	}
	if len(larger.Clients) != 6 || larger.PrivateKey != old.PrivateKey || larger.PublicKey != old.PublicKey || larger.ShortID != old.ShortID {
		t.Fatal("resize replaced listener identity")
	}
	if larger.Clients[0] != old.Clients[0] || larger.Clients[3] != old.Clients[1] {
		t.Fatal("existing users replaced")
	}
	small, e := Resize(larger, "profile", "panel", []string{"DIRECT", "RESIDENTIAL"}, 1)
	if e != nil || !reflect.DeepEqual(small, old) {
		t.Fatal("shrink did not retain stable identities", e)
	}
	reordered, e := Resize(larger, "profile", "panel", []string{"RESIDENTIAL", "DIRECT"}, 3)
	if e != nil || reordered.Clients[0] != larger.Clients[3] {
		t.Fatal("class reorder changed identity")
	}
	if _, e = Resize(old, "profile", "panel", []string{"DIRECT", "DIRECT"}, 1); e == nil {
		t.Fatal("duplicate classes accepted")
	}
}
