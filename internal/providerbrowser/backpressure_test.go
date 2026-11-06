package providerbrowser

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestBrowserTunnelSlowReceiverPreservesAssets(t *testing.T) {
	for _, pause := range []time.Duration{0, 200 * time.Millisecond} {
		t.Run(pause.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			l, r := net.Pipe()
			a, b := NewPeer(ctx, l), NewPeer(ctx, r)
			defer a.Close()
			defer b.Close()
			remote, producer := net.Pipe()
			defer remote.Close()
			defer producer.Close()
			a.Dial = func(context.Context, string) (net.Conn, error) { return remote, nil }
			a.Handle = func(context.Context, Action) (State, error) { return State{Title: "responsive"}, nil }
			a.Start()
			b.Start()
			c, e := b.Open(ctx, "fixture.example:443")
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			payload := bytes.Repeat([]byte("image and JavaScript asset bytes\x00"), 100000)
			sent := make(chan error, 1)
			go func() { _, e := producer.Write(payload); sent <- e; producer.Close() }()
			if pause > 0 {
				select {
				case e := <-sent:
					t.Fatalf("asset transfer ended while receiver paused: %v", e)
				case <-time.After(pause):
				}
				actionCtx, stop := context.WithTimeout(ctx, time.Second)
				s, e := b.Call(actionCtx, Action{Kind: "frame"})
				stop()
				if e != nil || s.Title != "responsive" {
					t.Fatalf("slow asset blocked controls: %v", e)
				}
			}
			c.SetReadDeadline(time.Now().Add(5 * time.Second))
			got, e := io.ReadAll(c)
			if e != nil || !bytes.Equal(got, payload) {
				t.Fatalf("asset truncated: got=%d want=%d error=%v", len(got), len(payload), e)
			}
			if e := <-sent; e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestBrowserTunnelCloseReleasesBackpressure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	l, r := net.Pipe()
	a, b := NewPeer(ctx, l), NewPeer(ctx, r)
	defer a.Close()
	defer b.Close()
	remote, producer := net.Pipe()
	defer remote.Close()
	defer producer.Close()
	a.Dial = func(context.Context, string) (net.Conn, error) { return remote, nil }
	a.Start()
	b.Start()
	c, e := b.Open(ctx, "fixture.example:443")
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	done := make(chan struct{})
	go func() { producer.Write(bytes.Repeat([]byte("asset"), 1000000)); close(done) }()
	time.Sleep(50 * time.Millisecond)
	b.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("backpressured producer survived session close")
	}
}
