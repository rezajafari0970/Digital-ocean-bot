package sanaei

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEconomySanaeiTransportIgnoresProxyEnvironment(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(key, "http://127.0.0.1:1")
	}
	c, err := NewAPIClient("http://example.test", Credentials{Username: "u", Password: "p"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tr, ok := c.HTTP.Transport.(*http.Transport)
	if !ok || tr.Proxy != nil {
		t.Fatal("panel inherited environment proxy")
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer s.Close()
	resp, err := c.HTTP.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatal(resp.StatusCode)
	}
	conn, err := tr.DialContext(context.Background(), "tcp", strings.TrimPrefix(s.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if conn.RemoteAddr().(*net.TCPAddr).IP.To4() == nil {
		t.Fatal("not IPv4")
	}
	if (Driver{}).client().Transport != directPanelTransport {
		t.Fatal("driver path is not explicit direct")
	}
}
