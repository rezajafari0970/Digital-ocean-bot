package auth

import "testing"

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("this-is-a-strong-password")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "this-is-a-strong-password" {
		t.Fatal("plaintext password")
	}
}
func TestShortPasswordRejected(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("expected rejection")
	}
}
func TestRBAC(t *testing.T) {
	if !(Principal{Role: Admin}).CanAdmin() {
		t.Fatal("admin")
	}
	if (Principal{Role: Viewer}).CanWrite() {
		t.Fatal("viewer write")
	}
	if !(Principal{Role: Operator}).CanWrite() {
		t.Fatal("operator")
	}
}
