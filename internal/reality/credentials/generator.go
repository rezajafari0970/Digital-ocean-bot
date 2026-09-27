package credentials

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidKeyOutput = errors.New(
	"invalid xray x25519 output",
)

type KeyPair struct {
	Private []byte

	Public string
}

func ParseX25519(
	output string,
) (KeyPair, error) {

	var (
		privateKey string
		publicKey  string
	)

	for _, line := range strings.Split(
		output,
		"\n",
	) {

		line = strings.TrimSpace(
			line,
		)

		switch {

		case strings.HasPrefix(
			line,
			"PrivateKey:",
		):

			privateKey =
				strings.TrimSpace(
					strings.TrimPrefix(
						line,
						"PrivateKey:",
					),
				)

		case strings.HasPrefix(
			line,
			"Password (PublicKey):",
		):

			publicKey =
				strings.TrimSpace(
					strings.TrimPrefix(
						line,
						"Password (PublicKey):",
					),
				)

		case strings.HasPrefix(
			line,
			"Password:",
		):

			publicKey =
				strings.TrimSpace(
					strings.TrimPrefix(
						line,
						"Password:",
					),
				)

		case strings.HasPrefix(
			line,
			"PublicKey:",
		):

			publicKey =
				strings.TrimSpace(
					strings.TrimPrefix(
						line,
						"PublicKey:",
					),
				)
		}
	}

	if privateKey == "" ||
		publicKey == "" {

		return KeyPair{},
			ErrInvalidKeyOutput
	}

	return KeyPair{
		Private: []byte(
			privateKey,
		),

		Public: publicKey,
	}, nil
}

func UUID() (
	string,
	error,
) {

	value := make(
		[]byte,
		16,
	)

	if _, err := rand.Read(
		value,
	); err != nil {

		return "", err
	}

	// UUID version 4.
	value[6] =
		(value[6] & 0x0f) |
			0x40

	// RFC 4122 variant.
	value[8] =
		(value[8] & 0x3f) |
			0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",

		value[0:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:16],
	), nil
}

func ShortID(
	size int,
) (string, error) {

	if size < 1 ||
		size > 8 {

		return "",
			errors.New(
				"invalid short id size",
			)
	}

	value := make(
		[]byte,
		size,
	)

	if _, err := rand.Read(
		value,
	); err != nil {

		return "", err
	}

	return hex.EncodeToString(
		value,
	), nil
}

func Wipe(
	value []byte,
) {

	for i := range value {
		value[i] = 0
	}
}
