package digitalocean

import "testing"

func TestParseAPIError(t *testing.T) {
	code, msg := parseAPIError([]byte(`{"id":"unprocessable_entity","message":"region is unavailable"}`))
	if code != "unprocessable_entity" || msg != "region is unavailable" {
		t.Fatalf("code=%q msg=%q", code, msg)
	}
}

func TestParseAPIErrorUsesCode(t *testing.T) {
	code, msg := parseAPIError([]byte(`{"code":"rate_limit","message":"slow down"}`))
	if code != "rate_limit" || msg != "slow down" {
		t.Fatalf("code=%q msg=%q", code, msg)
	}
}

func TestParseAPIErrorRejectsNonJSON(t *testing.T) {
	code, msg := parseAPIError([]byte("<html>proxy error</html>"))
	if code != "" || msg != "" {
		t.Fatalf("unexpected parse: %q %q", code, msg)
	}
}
