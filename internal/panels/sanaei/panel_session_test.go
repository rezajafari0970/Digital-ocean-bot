package sanaei

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestPanelSessionSnapshotLoadsOnce(t *testing.T) {
	var n atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"obj":[]}`))
	}))
	defer ts.Close()
	c := &APIClient{BaseURL: ts.URL, HTTP: ts.Client()}
	s := NewPanelSession(c)
	if _, e := s.Snapshot(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Snapshot(context.Background()); e != nil {
		t.Fatal(e)
	}
	if n.Load() != 1 {
		t.Fatalf("requests=%d", n.Load())
	}
	s.Invalidate()
	if _, e := s.Snapshot(context.Background()); e != nil {
		t.Fatal(e)
	}
	if n.Load() != 2 {
		t.Fatalf("requests after invalidate=%d", n.Load())
	}
}
