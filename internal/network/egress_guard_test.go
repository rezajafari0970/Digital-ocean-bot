package network

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type seqRT struct{ n int }

func (s *seqRT) RoundTrip(*http.Request) (*http.Response, error) {
	s.n++
	v := "1.1.1.1"
	if s.n > 1 {
		v = "2.2.2.2"
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(v)), Header: make(http.Header)}, nil
}
func TestEgressGuardDetectsChange(t *testing.T) {
	g := EgressGuard{Client: &http.Client{Transport: &seqRT{}}}
	if _, e := g.Observe(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e := g.Observe(context.Background()); !errorsIs(e, ErrEgressChanged) {
		t.Fatalf("%v", e)
	}
}
func errorsIs(a, b error) bool { return a == b }
