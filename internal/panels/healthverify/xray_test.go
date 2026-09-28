package healthverify

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	out string
	err error
	cmd string
}

func (f *fakeRunner) Run(_ context.Context, c string) (string, error) { f.cmd = c; return f.out, f.err }

func valid() RealityClient {
	return RealityClient{UUID: "eb44dbca-6064-4085-8145-ca022e5b2f65", Host: "167.172.28.228", Port: 443, SNI: "www.microsoft.com", PublicKey: "public", ShortID: "04a54a16"}
}

func TestProbePass(t *testing.T) {
	f := &fakeRunner{out: "TUNNEL_HTTPS=PASS\nip=167.172.28.228\nloc=US\n"}
	r, e := Probe(context.Background(), f, valid())
	if e != nil || !r.Healthy || r.ExitIP != "167.172.28.228" {
		t.Fatalf("%+v %v", r, e)
	}
	if !strings.Contains(f.cmd, "xtls-rprx-vision") && strings.Contains(f.cmd, "eb44dbca") {
		t.Fatal("raw credentials leaked into shell command")
	}
}
func TestProbeRejectsWrongExit(t *testing.T) {
	f := &fakeRunner{out: "TUNNEL_HTTPS=PASS\nip=1.1.1.1\n"}
	if _, e := Probe(context.Background(), f, valid()); !errors.Is(e, ErrFailed) {
		t.Fatalf("%v", e)
	}
}
func TestProbeRejectsTunnelFailure(t *testing.T) {
	f := &fakeRunner{out: "ip=167.172.28.228\n"}
	if _, e := Probe(context.Background(), f, valid()); !errors.Is(e, ErrFailed) {
		t.Fatalf("%v", e)
	}
}
