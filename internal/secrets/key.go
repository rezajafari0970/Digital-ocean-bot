package secrets

import (
	"encoding/base64"
	"errors"
	"os"
	"strings"
)

var ErrMasterKeyUnavailable = errors.New("master key unavailable")

func LoadMasterKey() ([]byte, error) {
	if path := strings.TrimSpace(os.Getenv("MASTER_KEY_FILE")); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, ErrMasterKeyUnavailable
		}
		return decodeMasterKey(strings.TrimSpace(string(b)))
	}
	if value := strings.TrimSpace(os.Getenv("MASTER_KEY_B64")); value != "" {
		return decodeMasterKey(value)
	}
	return nil, ErrMasterKeyUnavailable
}

func decodeMasterKey(v string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(v)
	if err != nil || len(b) != 32 {
		return nil, ErrInvalidMasterKey
	}
	return b, nil
}
