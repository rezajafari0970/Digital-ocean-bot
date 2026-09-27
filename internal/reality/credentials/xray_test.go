package credentials

import (
	"context"
	"testing"
)

func TestXrayGenerator(t *testing.T) {
	g := XrayGenerator{Run: func(context.Context, string) (string, error) { return "PrivateKey: p\nPassword: q\nHash32: h\n", nil }}
	k, e := g.Generate(context.Background())
	if e != nil || string(k.Private) != "p" || k.Public != "q" {
		t.Fatalf("%+v %v", k, e)
	}
}
