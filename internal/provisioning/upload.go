package provisioning

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var ErrSCPProtocol = errors.New("scp protocol error")

type scpAckError struct {
	Code    byte
	Message string
}

func (e scpAckError) Error() string {
	return fmt.Sprintf("%v: code=%d message=%s", ErrSCPProtocol, e.Code, e.Message)
}
func (e scpAckError) Unwrap() error { return ErrSCPProtocol }

func readSCPAck(r *bufio.Reader, stage string) error {
	b, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("%s ack: %w", stage, err)
	}
	if b == 0 {
		return nil
	}
	if b != 1 && b != 2 {
		return fmt.Errorf("%s ack: %w: unexpected code=%d", stage, ErrSCPProtocol, b)
	}
	msg, readErr := r.ReadString('\n')
	msg = strings.TrimSpace(msg)
	if len(msg) > 512 {
		msg = msg[len(msg)-512:]
	}
	if readErr != nil && readErr != io.EOF {
		return fmt.Errorf("%s ack: code=%d: %w", stage, b, readErr)
	}
	return fmt.Errorf("%s ack: %w", stage, scpAckError{Code: b, Message: msg})
}

func scpStageError(ctx context.Context, stage string, err error, started bool) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		if started {
			return fmt.Errorf("%s: %w", stage, errors.Join(ErrCommandOutcomeUnknown, ctxErr))
		}
		return fmt.Errorf("%s: %w", stage, ctxErr)
	}
	return fmt.Errorf("%s: %w", stage, err)
}

func (s SSHClient) Upload(ctx context.Context, target Target, privateKey []byte, localPath, remotePath string, mode os.FileMode) error {
	client, err := s.connect(ctx, target, privateKey)
	if err != nil {
		return err
	}
	defer client.Close()
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}
	if err = session.Start("scp -qt " + shellArg(remotePath)); err != nil {
		return scpStageError(ctx, "scp-start", err, false)
	}
	started := true
	guardDone := make(chan struct{})
	defer close(guardDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
			_ = client.Close()
		case <-guardDone:
		}
	}()
	ack := bufio.NewReader(stdout)
	if err = readSCPAck(ack, "scp-start"); err != nil {
		_ = session.Close()
		return scpStageError(ctx, "scp-start-ack", err, started)
	}
	if _, err = fmt.Fprintf(stdin, "C%04o %d payload\n", mode.Perm(), info.Size()); err != nil {
		return fmt.Errorf("scp-header write: %w", err)
	}
	if err = readSCPAck(ack, "scp-header"); err != nil {
		_ = session.Close()
		return scpStageError(ctx, "scp-header-ack", err, started)
	}
	if _, err = io.Copy(stdin, f); err != nil {
		return scpStageError(ctx, "scp-data-write", err, started)
	}
	if _, err = stdin.Write([]byte{0}); err != nil {
		return scpStageError(ctx, "scp-data-finalize", err, started)
	}
	if err = readSCPAck(ack, "scp-data"); err != nil {
		_ = session.Close()
		return scpStageError(ctx, "scp-data-ack", err, started)
	}
	if err = stdin.Close(); err != nil {
		return scpStageError(ctx, "scp-close", err, started)
	}
	if err = session.Wait(); err != nil {
		b, _ := io.ReadAll(io.LimitReader(stderr, 1024))
		if ctx.Err() != nil {
			return scpStageError(ctx, "scp-wait", err, started)
		}
		return fmt.Errorf("scp-wait: %w; diagnostic=%s", err, strings.TrimSpace(string(b)))
	}
	return nil
}

func shellArg(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
