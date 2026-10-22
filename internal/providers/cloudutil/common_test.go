package cloudutil

import (
	"context"
	"net/http"
	"testing"
)

type credential string

func (c credential) Get(context.Context) ([]byte, error) { return []byte(c), nil }
func TestCredentialStrictAndHTTPIsolation(t *testing.T) {
	for _, x := range []string{`{"key":"x"} {}`, `{"unknown":"x"}`, `null`} {
		var v struct {
			Key string `json:"key"`
		}
		e := ReadCredential(context.Background(), credential(x), &v)
		if e == nil && x != "null" {
			t.Fatal(x)
		}
	}
	h := &http.Client{}
	c := HTTPClient(h)
	if h.CheckRedirect != nil || c == h || c.CheckRedirect == nil {
		t.Fatal("client isolation broken")
	}
	if _, e := CloudInit([]string{"ssh-ed25519 garbage\ncommand=oops"}); e == nil {
		t.Fatal("invalid SSH key accepted")
	}
	if PublicIPv4("127.0.0.1") != "" || PublicIPv4("10.1.2.3") != "" || PublicIPv4("8.8.8.8") == "" {
		t.Fatal("IP classification")
	}
}
