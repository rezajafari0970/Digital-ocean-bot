package provisioning

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadSCPAckOK(t *testing.T) {
	if err := readSCPAck(bufio.NewReader(bytes.NewReader([]byte{0})), "start"); err != nil {
		t.Fatal(err)
	}
}
func TestReadSCPAckRemoteError(t *testing.T) {
	for _, code := range []byte{1, 2} {
		err := readSCPAck(bufio.NewReader(bytes.NewReader(append([]byte{code}, []byte("permission denied\n")...))), "header")
		if err == nil || !errors.Is(err, ErrSCPProtocol) {
			t.Fatalf("code=%d err=%v", code, err)
		}
		if !strings.Contains(err.Error(), "header") || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("bad diagnostic %v", err)
		}
	}
}
func TestReadSCPAckUnexpectedCode(t *testing.T) {
	err := readSCPAck(bufio.NewReader(bytes.NewReader([]byte{9})), "data")
	if err == nil || !errors.Is(err, ErrSCPProtocol) {
		t.Fatalf("err=%v", err)
	}
}
func TestReadSCPAckEOF(t *testing.T) {
	err := readSCPAck(bufio.NewReader(bytes.NewReader(nil)), "start")
	if err == nil || !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("err=%v", err)
	}
}
func TestShellArgEscapesQuote(t *testing.T) {
	got := shellArg("/tmp/a'b")
	if got != "'/tmp/a'\\''b'" {
		t.Fatalf("got %q", got)
	}
}

func TestSCPStageErrorCanceledAfterStartIsAmbiguous(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := scpStageError(ctx, "scp-data", io.EOF, true)
	if !errors.Is(err, ErrCommandOutcomeUnknown) {
		t.Fatalf("expected unknown outcome, got %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
}

func TestSCPStageErrorCanceledBeforeStartNotAmbiguous(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := scpStageError(ctx, "scp-start", io.EOF, false)
	if errors.Is(err, ErrCommandOutcomeUnknown) {
		t.Fatalf("unexpected unknown outcome: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
}
