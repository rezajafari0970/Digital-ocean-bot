package credentials

import (
	"regexp"
	"testing"
)

func TestParseCurrentXrayFormat(
	t *testing.T,
) {

	keyPair, err := ParseX25519(
		"PrivateKey: private-value\n" +
			"Password: public-value\n" +
			"Hash32: ignored\n",
	)

	if err != nil {
		t.Fatal(err)
	}

	if string(keyPair.Private) !=
		"private-value" {

		t.Fatal(
			"private key parse failed",
		)
	}

	if keyPair.Public !=
		"public-value" {

		t.Fatal(
			"public key parse failed",
		)
	}
}

func TestUUIDv4(
	t *testing.T,
) {

	uuid, err := UUID()

	if err != nil {
		t.Fatal(err)
	}

	pattern :=
		regexp.MustCompile(
			`^[0-9a-f]{8}-` +
				`[0-9a-f]{4}-` +
				`4[0-9a-f]{3}-` +
				`[89ab][0-9a-f]{3}-` +
				`[0-9a-f]{12}$`,
		)

	if !pattern.MatchString(
		uuid,
	) {
		t.Fatalf(
			"invalid UUID: %s",
			uuid,
		)
	}
}

func TestShortID(
	t *testing.T,
) {

	shortID, err :=
		ShortID(
			4,
		)

	if err != nil {
		t.Fatal(err)
	}

	if len(shortID) != 8 {
		t.Fatalf(
			"unexpected short id length: %d",
			len(shortID),
		)
	}
}

func TestWipe(
	t *testing.T,
) {

	value := []byte(
		"secret",
	)

	Wipe(
		value,
	)

	for _, item := range value {
		if item != 0 {
			t.Fatal(
				"private data was not wiped",
			)
		}
	}
}
