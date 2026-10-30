package providerbrowser

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestBrowserTunnelIsolationAndClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	left, right := net.Pipe()
	a, b := NewPeer(ctx, left), NewPeer(ctx, right)
	defer a.Close()
	defer b.Close()
	remote, echo := net.Pipe()
	defer remote.Close()
	defer echo.Close()
	a.Dial = func(_ context.Context, address string) (net.Conn, error) {
		if address != "fixture.example:443" {
			t.Errorf("unexpected destination %q", address)
		}
		return remote, nil
	}
	a.Start()
	b.Start()
	go io.Copy(echo, echo)
	c, e := b.Open(ctx, "fixture.example:443")
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	data := []byte("TLS bytes and cookies belong to this tunnel")
	go c.Write(data)
	got := make([]byte, len(data))
	if _, e = io.ReadFull(c, got); e != nil || string(got) != string(data) {
		t.Fatal(string(got), e)
	}
	a.Close()
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	if _, e = c.Read(make([]byte, 1)); e == nil {
		t.Fatal("tunnel survived route shutdown")
	}
}
func TestBrowserProtocolRejectsOversizeAndCommands(t *testing.T) {
	a, b := net.Pipe()
	p := NewPeer(context.Background(), a)
	p.Start()
	defer p.Close()
	defer b.Close()
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], maxPacket+1)
	if _, e := b.Write(h[:]); e != nil {
		t.Fatal(e)
	}
	select {
	case <-p.Done():
	case <-time.After(time.Second):
		t.Fatal("oversized packet not closed")
	}
	for _, u := range []string{"http://provider.example/", "file:///etc/passwd", "javascript:alert(1)", "https://user:secret@provider.example/", "https://provider.example:123/"} {
		if ValidURL(u) {
			t.Fatal("unsafe entrypoint", u)
		}
	}
	if !ValidURL("https://my.vultr.com/") {
		t.Fatal("official HTTPS entry rejected")
	}
}
func TestBrowserWorkerRejectsHostNetwork(t *testing.T) {
	interfaces, e := net.Interfaces()
	if e != nil {
		t.Fatal(e)
	}
	for _, i := range interfaces {
		if i.Flags&net.FlagLoopback == 0 {
			if VerifyIsolation() == nil {
				t.Fatal("host network accepted")
			}
			return
		}
	}
	t.Skip("already inside an isolated network namespace")
}
