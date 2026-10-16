package provisioning

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSCPUploadDrainsLargeStderr(t *testing.T) {
	payload := bytes.Repeat([]byte("artifact-data"), 450000)
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	private := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw})
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	completed := make(chan error, 1)
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			completed <- e
			return
		}
		defer conn.Close()
		cfg := &ssh.ServerConfig{NoClientAuth: true}
		cfg.AddHostKey(signer)
		server, channels, requests, e := ssh.NewServerConn(conn, cfg)
		if e != nil {
			completed <- e
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		ch, ok := <-channels
		if !ok {
			completed <- fmt.Errorf("missing channel")
			return
		}
		channel, reqs, e := ch.Accept()
		if e != nil {
			completed <- e
			return
		}
		defer channel.Close()
		request, ok := <-reqs
		if !ok || request.Type != "exec" {
			completed <- fmt.Errorf("missing exec")
			return
		}
		_ = request.Reply(true, nil)
		go ssh.DiscardRequests(reqs)
		if _, e = channel.Write([]byte{0}); e != nil {
			completed <- e
			return
		}
		reader := bufio.NewReader(channel)
		header, e := reader.ReadString('\n')
		if e != nil {
			completed <- e
			return
		}
		var mode, size int
		var name string
		if _, e = fmt.Sscanf(header, "C%o %d %s", &mode, &size, &name); e != nil || size != len(payload) {
			completed <- fmt.Errorf("invalid header %q", header)
			return
		}
		if _, e = channel.Write([]byte{0}); e != nil {
			completed <- e
			return
		}
		// More than the SSH channel window: deferring stderr reads until Wait
		// deadlocks the sender and receiver before either can finish its payload.
		if _, e = io.Copy(channel.Stderr(), io.LimitReader(strings.NewReader(strings.Repeat("diagnostic\n", 400000)), 4<<20)); e != nil {
			completed <- e
			return
		}
		hash := sha256.New()
		if _, e = io.CopyN(hash, reader, int64(size)); e != nil {
			completed <- e
			return
		}
		want := sha256.Sum256(payload)
		if !bytes.Equal(hash.Sum(nil), want[:]) {
			completed <- fmt.Errorf("payload mismatch")
			return
		}
		end, e := reader.ReadByte()
		if e != nil || end != 0 {
			completed <- fmt.Errorf("missing data terminator")
			return
		}
		if _, e = channel.Write([]byte{0}); e != nil {
			completed <- e
			return
		}
		if _, e = io.Copy(io.Discard, reader); e != nil {
			completed <- e
			return
		}
		_, e = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
		completed <- e
	}()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	n, _ := strconv.Atoi(port)
	path := filepath.Join(t.TempDir(), "artifact")
	if err = os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = (SSHClient{HostKeys: guardPins{}}).Upload(ctx, Target{Host: "127.0.0.1", Port: n, User: "fixture"}, private, path, "/tmp/fixture", 0600)
	select {
	case remote := <-completed:
		if remote != nil && err == nil {
			t.Fatal(remote)
		}
	case <-time.After(time.Second):
		t.Fatal("SSH fixture did not terminate")
	}
	if err != nil {
		t.Fatalf("large upload blocked behind stderr: %v", err)
	}
}

func TestSCPDiagnosticBoundedAndContinues(t *testing.T) {
	var d scpDiagnostic
	for i := 0; i < 8; i++ {
		p := bytes.Repeat([]byte{'x'}, 4096)
		n, e := d.Write(p)
		if n != len(p) || e != nil {
			t.Fatal(n, e)
		}
	}
	if len(d.String()) != 1024 || d.String() != strings.Repeat("x", 1024) {
		t.Fatal("diagnostic not bounded")
	}
}
