package provisioning

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

type guardPins struct{}

func (guardPins) VerifyOrPin(context.Context, Target, string) error { return nil }
func guardSSH(t *testing.T, mode string) (Target, []byte) {
	t.Helper()
	_, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := x509.MarshalPKCS8PrivateKey(key)
	private := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw})
	signer, _ := ssh.NewSignerFromKey(key)
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer c.Close()
				if mode == "silent" {
					<-stop
					return
				}
				cfg := &ssh.ServerConfig{NoClientAuth: true}
				cfg.AddHostKey(signer)
				conn, chs, rs, e := ssh.NewServerConn(c, cfg)
				if e != nil {
					return
				}
				defer conn.Close()
				go ssh.DiscardRequests(rs)
				for ch := range chs {
					if mode == "channel" {
						<-stop
						return
					}
					channel, reqs, e := ch.Accept()
					if e != nil {
						return
					}
					for req := range reqs {
						if mode == "start" {
							<-stop
							channel.Close()
							return
						}
						if req.Type == "exec" {
							req.Reply(true, nil)
							if mode == "input" {
								data, err := io.ReadAll(channel)
								if err != nil || !bytes.Equal(data, []byte("private-stdin-fixture")) {
									t.Error("stdin was not delivered intact")
								}
								var command struct{ Command string }
								ssh.Unmarshal(req.Payload, &command)
								if command.Command != "fixed-read-only-command" {
									t.Error("input leaked into shell command")
								}
								channel.Write([]byte("input-verified"))
							} else {
								channel.Write([]byte("ok"))
							}
							channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
							channel.Close()
							return
						}
						req.Reply(false, nil)
					}
				}
			}()
		}
	}()
	t.Cleanup(func() { close(stop); l.Close(); wg.Wait() })
	_, port, _ := net.SplitHostPort(l.Addr().String())
	n, _ := strconv.Atoi(port)
	return Target{Host: "127.0.0.1", Port: n, User: "fixture"}, private
}
func TestSSHGuardCancellationFaults(t *testing.T) {
	for _, mode := range []string{"silent", "channel", "start"} {
		t.Run(mode, func(t *testing.T) {
			target, key := guardSSH(t, mode)
			client := SSHClient{HostKeys: guardPins{}, Timeout: time.Second}
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			at := time.Now()
			_, e := client.Run(ctx, target, key, "true")
			if e == nil || time.Since(at) > time.Second {
				t.Fatalf("run not bounded %v %v", time.Since(at), e)
			}
			if mode == "silent" {
				at = time.Now()
				e = (SSHClient{HostKeys: guardPins{}, Timeout: 150 * time.Millisecond}).Wait(context.Background(), target, key)
				if e == nil || time.Since(at) > time.Second {
					t.Fatal("wait overall timeout ignored", e)
				}
			}
			if mode != "silent" {
				file := filepath.Join(t.TempDir(), "artifact")
				os.WriteFile(file, []byte("x"), 0600)
				ctx2, cancel2 := context.WithTimeout(context.Background(), 150*time.Millisecond)
				defer cancel2()
				at = time.Now()
				e = client.Upload(ctx2, target, key, file, "/tmp/fixture", 0600)
				if e == nil || time.Since(at) > time.Second {
					t.Fatal("upload cancellation unbounded", e)
				}
			}
		})
	}
}
func TestSSHGuardNormalAndHostKeyVerification(t *testing.T) {
	target, key := guardSSH(t, "normal")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, e := (SSHClient{HostKeys: guardPins{}}).Run(ctx, target, key, "true")
	if e != nil || out != "ok" {
		t.Fatal(out, e)
	}
	target2, key2 := guardSSH(t, "normal")
	if _, e = (SSHClient{}).Run(ctx, target2, key2, "true"); e == nil {
		t.Fatal("missing host verifier accepted")
	}
}

func TestSSHInputUsesPinnedEncryptedStdin(t *testing.T) {
	target, key := guardSSH(t, "input")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, e := (SSHClient{HostKeys: guardPins{}}).RunInput(ctx, target, key, "fixed-read-only-command", []byte("private-stdin-fixture"))
	if e != nil || out.Stdout != "input-verified" {
		t.Fatal(out, e)
	}
	target2, key2 := guardSSH(t, "normal")
	if _, e = (SSHClient{}).RunInput(ctx, target2, key2, "fixed-read-only-command", []byte("private-stdin-fixture")); e == nil {
		t.Fatal("missing host verifier accepted")
	}
	target3, key3 := guardSSH(t, "start")
	bounded, stop := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer stop()
	at := time.Now()
	if _, e = (SSHClient{HostKeys: guardPins{}}).RunInput(bounded, target3, key3, "fixed-read-only-command", []byte("private-stdin-fixture")); e == nil || time.Since(at) > time.Second {
		t.Fatal("stdin session cancellation unbounded", e)
	}
}
