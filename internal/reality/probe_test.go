package reality

import "testing"

func TestValidateCandidate(t *testing.T) {
	if ValidateCandidate(Candidate{Target: "example.com", ServerName: "example.com", Port: 443}) != nil {
		t.Fatal("valid rejected")
	}
	if ValidateCandidate(Candidate{Target: "127.0.0.1", ServerName: "x", Port: 443}) == nil {
		t.Fatal("ip target accepted")
	}
}
