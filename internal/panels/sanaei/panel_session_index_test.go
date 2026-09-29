package sanaei

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPanelSessionBuildsAndClearsIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"obj":[{"id":7,"remark":"r","port":443}]}`))
	}))
	defer srv.Close()
	c := &APIClient{BaseURL: srv.URL, HTTP: srv.Client()}
	s := NewPanelSession(c)
	x, e := s.Index(context.Background())
	if e != nil || x == nil || x.ByRemoteID[7] == nil || x.ByPort[443] == nil || x.ByRemark["r"] == nil {
		t.Fatalf("index=%+v err=%v", x, e)
	}
	s.Invalidate()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index != nil || s.loaded {
		t.Fatal("invalidate must clear snapshot and index")
	}
}
