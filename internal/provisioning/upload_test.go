package provisioning

import (
	"bufio"
	"bytes"
	"errors"
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
