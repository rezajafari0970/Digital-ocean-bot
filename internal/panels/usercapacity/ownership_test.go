package usercapacity

import "testing"

func TestOwnershipMarkerRequiresExactGenerationPrefix(t *testing.T) {
	marker := "p12345678-i1"
	id := "11111111-1111-4111-8111-111111111111"
	email := ownershipEmail(marker, id)
	if !ownershipMatches(email, marker) {
		t.Fatalf("generated email %q must match marker", email)
	}
	if ownershipMatches(email, "p87654321-i1") {
		t.Fatal("different generation marker must not match")
	}
}

func TestOwnershipEmailContainsStableMarkerAndClientSuffix(t *testing.T) {
	got := ownershipEmail("p12345678-i1", "abcdef12-3456-4789-8123-456789abcdef")
	want := "u-p12345678-i1-abcdef12"
	if got != want {
		t.Fatalf("email=%q want %q", got, want)
	}
}

func TestObservedClientsSupportsSanaeiStringSettings(t *testing.T) {
	raw := []byte(`{"settings":"{\"clients\":[{\"id\":\"u1\",\"email\":\"u-marker-12345678\"}]}"}`)
	got, err := observedClients(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got["u1"] != "u-marker-12345678" {
		t.Fatalf("observed=%v", got)
	}
}

func TestRollbackMarkerDoesNotMatchOrdinaryUser(t *testing.T) {
	if ownershipMatches("ordinary-user", "p12345678-i1") {
		t.Fatal("ordinary user must never match owned generation")
	}
}
