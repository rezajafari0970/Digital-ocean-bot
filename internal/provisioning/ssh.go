package provisioning

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

var ErrSSHNotReady = errors.New("ssh not ready")
var ErrSSHCommand = errors.New("ssh command failed")
var ErrCommandOutcomeUnknown = errors.New("remote command outcome unknown")

type CommandResult struct {
	Stdout, Stderr string
	ExitCode       *int
	Signal         string
}
type ProbeObserver func(error, time.Duration)
type SSHClient struct {
	Timeout  time.Duration
	HostKeys HostKeyPins
}

func (s SSHClient) Wait(ctx context.Context, t Target, key []byte) error {
	return s.WaitObserved(ctx, t, key, nil)
}
func (s SSHClient) WaitObserved(ctx context.Context, t Target, key []byte, observe ProbeObserver) error {
	return s.WaitStages(ctx, t, key, observe, nil)
}
func (s SSHClient) WaitStages(ctx context.Context, t Target, key []byte, observe ProbeObserver, stages StageObserver) error {
	interval := time.Second
	var last error
	for {
		started := time.Now()
		err := s.probeStages(ctx, t, key, stages)
		if observe != nil {
			observe(err, time.Since(started))
		}
		if err == nil {
			return nil
		}
		last = err
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: last=%v", ctx.Err(), last)
		case <-time.After(interval):
		}
		if interval < 8*time.Second {
			interval *= 2
		}
	}
}
func (s SSHClient) Probe(ctx context.Context, t Target, key []byte) error {
	return s.probeStages(ctx, t, key, nil)
}
func (s SSHClient) probeStages(ctx context.Context, t Target, key []byte, observe StageObserver) error {
	client, err := s.connectStages(ctx, t, key, observe)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSSHNotReady, err)
	}
	defer client.Close()
	start := time.Now()
	session, err := client.NewSession()
	emitStage(observe, StageSession, start, err)
	if err != nil {
		return fmt.Errorf("%w: session: %v", ErrSSHNotReady, err)
	}
	defer session.Close()
	start = time.Now()
	err = runSessionContext(ctx, session, "true")
	emitStage(observe, StageProbe, start, err)
	if err != nil {
		return fmt.Errorf("%w: probe: %v", ErrSSHNotReady, err)
	}
	return nil
}
func (s SSHClient) Run(ctx context.Context, t Target, key []byte, command string) (string, error) {
	r, e := s.RunDetailed(ctx, t, key, command)
	return r.Stdout + r.Stderr, e
}
func (s SSHClient) RunDetailed(ctx context.Context, t Target, key []byte, command string) (CommandResult, error) {
	return s.RunDetailedObserved(ctx, t, key, command, nil)
}
func (s SSHClient) RunDetailedObserved(ctx context.Context, t Target, key []byte, command string, observe StageObserver) (CommandResult, error) {
	client, err := s.connectStages(ctx, t, key, observe)
	if err != nil {
		return CommandResult{}, err
	}
	defer client.Close()
	start := time.Now()
	session, err := client.NewSession()
	emitStage(observe, StageSession, start, err)
	if err != nil {
		return CommandResult{}, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	start = time.Now()
	err = runSessionContext(ctx, session, command)
	emitStage(observe, StageCommand, start, err)
	res := CommandResult{Stdout: tailDiagnostic(stdout.String()), Stderr: tailDiagnostic(stderr.String())}
	if err != nil {
		var exit *ssh.ExitError
		if errors.As(err, &exit) {
			code := exit.ExitStatus()
			res.ExitCode = &code
			res.Signal = exit.Signal()
		}
		return res, fmt.Errorf("%w: %w", ErrSSHCommand, err)
	}
	code := 0
	res.ExitCode = &code
	return res, nil
}
func (s SSHClient) connect(ctx context.Context, t Target, key []byte) (*ssh.Client, error) {
	return s.connectStages(ctx, t, key, nil)
}
func (s SSHClient) connectStages(ctx context.Context, t Target, key []byte, observe StageObserver) (*ssh.Client, error) {
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("ssh private key: %w", err)
	}
	port := t.Port
	if port == 0 {
		port = 22
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	hostKeyCallback := func(_ string, _ net.Addr, _ ssh.PublicKey) error { return ErrHostKeyVerifierMissing }
	if s.HostKeys != nil {
		hostKeyCallback = func(_ string, _ net.Addr, key ssh.PublicKey) error {
			return s.HostKeys.VerifyOrPin(ctx, t, ssh.FingerprintSHA256(key))
		}
	}
	config := &ssh.ClientConfig{User: t.User, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: hostKeyCallback, Timeout: timeout}
	addr := net.JoinHostPort(t.Host, fmt.Sprintf("%d", port))
	dialer := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	emitStage(observe, StageTCP, start, err)
	if err != nil {
		return nil, fmt.Errorf("ssh tcp dial: %w", err)
	}
	start = time.Now()
	cc, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	emitStage(observe, StageHandshake, start, err)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}
	return ssh.NewClient(cc, chans, reqs), nil
}
func emitStage(observe StageObserver, stage ConnectionStage, start time.Time, err error) {
	if observe != nil {
		observe(StageObservation{Stage: stage, Duration: time.Since(start), Err: err})
	}
}

func runSessionContext(ctx context.Context, session *ssh.Session, command string) error {
	if err := session.Start(command); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = session.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		return errors.Join(ErrCommandOutcomeUnknown, ctx.Err())
	}
}
