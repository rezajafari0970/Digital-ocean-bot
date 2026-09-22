package redact

import "testing"

func TestSensitiveHeadersAreRedacted(t *testing.T) {
	if got := Header("Authorization", "Bearer secret"); got != "[REDACTED]" {
		t.Fatalf("got %q", got)
	}
	if got := Header("Cookie", "session=abc"); got != "[REDACTED]" {
		t.Fatalf("got %q", got)
	}
}

func TestBearerTokensAreRedactedFromText(t *testing.T) {
	got := Text("request failed: Bearer abc.def-123")
	if got != "request failed: Bearer [REDACTED]" {
		t.Fatalf("got %q", got)
	}
}
