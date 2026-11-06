package providerbrowser

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"
)

// Legacy peers ignore unknown JSON fields but reject unknown message kinds.
// Exercise both sides of a rolling API/shared-renderer upgrade using that wire contract.
func TestBrowserTunnelLegacyPeerCompatibility(t *testing.T) {
	read := func(t *testing.T, c net.Conn) message {
		t.Helper()
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		var h [4]byte
		if _, e := io.ReadFull(c, h[:]); e != nil {
			t.Fatal(e)
		}
		n := binary.BigEndian.Uint32(h[:])
		if n > maxPacket {
			t.Fatal("oversized frame")
		}
		raw := make([]byte, n)
		if _, e := io.ReadFull(c, raw); e != nil {
			t.Fatal(e)
		}
		var m message
		if e := json.Unmarshal(raw, &m); e != nil {
			t.Fatal(e)
		}
		return m
	}
	t.Run("new opener old gateway", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		l, r := net.Pipe()
		p, legacy := NewPeer(ctx, l), NewPeer(ctx, r)
		defer p.Close()
		defer legacy.Close()
		p.Start()
		type result struct {
			c net.Conn
			e error
		}
		opened := make(chan result, 1)
		go func() { c, e := p.Open(ctx, "fixture.example:443"); opened <- result{c, e} }()
		dial := read(t, r)
		if dial.Kind != "dial" || !dial.FlowControl {
			t.Fatal("capability missing")
		}
		if e := legacy.send(message{Kind: "opened", ID: dial.ID}); e != nil {
			t.Fatal(e)
		}
		out := <-opened
		if out.e != nil {
			t.Fatal(out.e)
		}
		defer out.c.Close()
		written := make(chan error, 1)
		go func() { _, e := out.c.Write([]byte("request")); written <- e }()
		req := read(t, r)
		if req.Kind != "data" || string(req.Data) != "request" {
			t.Fatalf("legacy request: %q", req.Kind)
		}
		if e := <-written; e != nil {
			t.Fatal(e)
		}
		if e := legacy.send(message{Kind: "data", ID: dial.ID, Data: []byte("response")}); e != nil {
			t.Fatal(e)
		}
		out.c.SetReadDeadline(time.Now().Add(time.Second))
		data := make([]byte, 8)
		if _, e := io.ReadFull(out.c, data); e != nil || string(data) != "response" {
			t.Fatal("legacy response", e)
		}
		out.c.Close()
		m := read(t, r)
		if m.Kind != "closed" {
			t.Fatalf("unsupported message sent to old gateway: %q", m.Kind)
		}
	})
	t.Run("old opener new gateway", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		l, r := net.Pipe()
		p, legacy := NewPeer(ctx, l), NewPeer(ctx, r)
		defer p.Close()
		defer legacy.Close()
		remote, echo := net.Pipe()
		defer remote.Close()
		defer echo.Close()
		p.Dial = func(context.Context, string) (net.Conn, error) { return remote, nil }
		p.Start()
		go io.Copy(echo, echo)
		if e := legacy.send(message{Kind: "dial", ID: "old1", Address: "fixture.example:443"}); e != nil {
			t.Fatal(e)
		}
		m := read(t, r)
		if m.Kind != "opened" || m.FlowControl {
			t.Fatal("legacy opening rejected")
		}
		if e := legacy.send(message{Kind: "data", ID: "old1", Data: []byte("echo")}); e != nil {
			t.Fatal(e)
		}
		m = read(t, r)
		if m.Kind != "data" || string(m.Data) != "echo" {
			t.Fatalf("unsupported message sent to old opener: %q", m.Kind)
		}
		if e := legacy.send(message{Kind: "closed", ID: "old1"}); e != nil {
			t.Fatal(e)
		}
		m = read(t, r)
		if m.Kind != "closed" {
			t.Fatalf("unsupported legacy close: %q", m.Kind)
		}
	})
}
