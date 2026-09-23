package adminapi

import (
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("code=%d", w.Code)
	}
}
func TestReadinessWithoutDatabase(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("code=%d", w.Code)
	}
}
