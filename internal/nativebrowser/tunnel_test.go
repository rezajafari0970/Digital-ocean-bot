package nativebrowser

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func request(key [16]byte) []byte {
	b := append([]byte{0}, key[:]...)
	b = append(b, 0, 1, 1, 187, 2, 11)
	return append(b, []byte("example.com")...)
}
func TestNativeHandshakeBoundaries(t *testing.T) {
	key := [16]byte{7, 2, 4}
	valid := request(key)
	for n := 0; n < len(valid); n++ {
		if _, e := Destination(bytes.NewReader(valid[:n]), key); e == nil {
			t.Fatalf("truncated header %d accepted", n)
		}
	}
	for _, change := range []struct {
		offset int
		value  byte
	}{{0, 1}, {1, 8}, {17, 1}, {18, 2}, {18, 3}, {18, 4}, {19, 0}, {21, 9}, {22, 0}, {23, '/'}} {
		b := bytes.Clone(valid)
		b[change.offset] = change.value
		if _, e := Destination(bytes.NewReader(b), key); e == nil {
			t.Fatalf("unsupported request accepted at %d", change.offset)
		}
	}
	r := bytes.NewReader(append(bytes.Clone(valid), []byte("TLS-untouched")...))
	dest, e := Destination(r, key)
	if e != nil || dest != "example.com:443" {
		t.Fatal(dest, e)
	}
	b, _ := io.ReadAll(r)
	if string(b) != "TLS-untouched" {
		t.Fatal("payload consumed")
	}
}
func TestNativeRelayCancellationAndNoDirectFallback(t *testing.T) {
	key := [16]byte{7, 2, 4}
	client, server := net.Pipe()
	upstream, target := net.Pipe()
	defer target.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	connected := make(chan struct{})
	go func() {
		done <- Relay(ctx, server, key, func(ctx context.Context, addr string) (net.Conn, error) {
			if addr != "example.com:443" {
				t.Error(addr)
			}
			return upstream, nil
		}, func() { close(connected) }, func(int64) {})
	}()
	client.SetDeadline(time.Now().Add(3 * time.Second))
	client.Write(request(key))
	var header [2]byte
	if _, e := io.ReadFull(client, header[:]); e != nil || header != [2]byte{} {
		t.Fatal(header, e)
	}
	select {
	case <-connected:
		t.Fatal("client marked ready without website bytes")
	default:
	}
	go func() { target.Write([]byte("server bytes")) }()
	b := make([]byte, 12)
	if _, e := io.ReadFull(client, b); e != nil || string(b) != "server bytes" {
		t.Fatal(string(b), e)
	}
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("missing response confirmation")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("revoked tunnel stayed open")
	}
	if _, e := client.Read(b); e == nil {
		t.Fatal("revoked connection readable")
	}
	client.Close()
	client, server = net.Pipe()
	defer client.Close()
	calls := 0
	go func() {
		done <- Relay(context.Background(), server, key, func(context.Context, string) (net.Conn, error) { calls++; return nil, context.DeadlineExceeded }, func() { t.Error("failed dial acknowledged") }, func(int64) {})
	}()
	client.SetDeadline(time.Now().Add(2 * time.Second))
	client.Write(request(key))
	if _, e := client.Read(b); e == nil {
		t.Fatal("failed route delivered bytes")
	}
	<-done
	if calls != 1 {
		t.Fatal("route fallback/retry", calls)
	}
}
