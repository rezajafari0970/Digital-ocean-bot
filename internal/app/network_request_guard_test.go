package app

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
)

type guardCountTransport struct{ calls atomic.Int32 }

func (r *guardCountTransport) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls.Add(1)
	return &http.Response{StatusCode: 204, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestAccountNetworkGuardBlocksBeforeNetwork(t *testing.T) {
	base := &guardCountTransport{}
	blocked := errors.New("network recovery active")
	tr := accountNetworkGuardTransport{
		Base:  base,
		Check: func(context.Context) error { return blocked },
	}
	req, _ := http.NewRequest(http.MethodGet, "https://example.invalid/", nil)
	if _, err := tr.RoundTrip(req); !errors.Is(err, blocked) {
		t.Fatalf("err=%v", err)
	}
	if base.calls.Load() != 0 {
		t.Fatalf("blocked request reached network: %d", base.calls.Load())
	}
}

func TestAccountNetworkGuardAllowsOnlyAfterCheck(t *testing.T) {
	base := &guardCountTransport{}
	tr := accountNetworkGuardTransport{Base: base, Check: func(context.Context) error { return nil }}
	req, _ := http.NewRequest(http.MethodGet, "https://example.invalid/", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil || resp.StatusCode != 204 {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
	if base.calls.Load() != 1 {
		t.Fatalf("calls=%d", base.calls.Load())
	}
}
