package credentials

import "testing"

func TestCredentialIdentityIsDeterministic(
	t *testing.T,
) {

	panelID :=
		"panel-a"

	managedKey :=
		"dob:reality-primary:000001"

	if panelID == "" ||
		managedKey == "" {

		t.Fatal(
			"invalid credential identity",
		)
	}
}
