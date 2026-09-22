package provisioning

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"net"
	"time"
)

var ErrSSHNotReady = errors.New("ssh not ready")
var ErrSSHCommand = errors.New("ssh command failed")

type SSHClient struct{ Timeout time.Duration }

func (s SSHClient) Wait(ctx context.Context, target Target, privateKey []byte) error {
	interval := time.Second
	for {
		if err := s.Probe(ctx, target, privateKey); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
		if interval < 8*time.Second {
			interval *= 2
		}
	}
}

func (s SSHClient) Probe(ctx context.Context, target Target, privateKey []byte) error {
	client, err := s.connect(ctx, target, privateKey)
	if err != nil {
		return ErrSSHNotReady
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return ErrSSHNotReady
	}
	defer session.Close()
	if err := session.Run("true"); err != nil {
		return ErrSSHNotReady
	}
	return nil
}

func (s SSHClient) Run(ctx context.Context, target Target, privateKey []byte, command string) (string, error) {
	client, err := s.connect(ctx, target, privateKey)
	if err != nil {
		return "", err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	var out bytes.Buffer
	session.Stdout = &out
	session.Stderr = &out
	if err := session.Run(command); err != nil {
		return out.String(), fmt.Errorf("%w", ErrSSHCommand)
	}
	return out.String(), nil
}

func (s SSHClient) connect(ctx context.Context, target Target, privateKey []byte) (*ssh.Client, error) {
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	port := target.Port
	if port == 0 {
		port = 22
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	config := &ssh.ClientConfig{User: target.User, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: timeout}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(target.Host, fmt.Sprintf("%d", port)))
	if err != nil {
		return nil, err
	}
	cc, chans, reqs, err := ssh.NewClientConn(conn, net.JoinHostPort(target.Host, fmt.Sprintf("%d", port)), config)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return ssh.NewClient(cc, chans, reqs), nil
}
