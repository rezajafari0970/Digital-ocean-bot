package network

import (
	"encoding/hex"
	"errors"
	"strings"
)

// CanonicalAccountID makes every PostgreSQL UUID spelling use one lock key.
func CanonicalAccountID(id string) (string, error) {
	raw := strings.TrimSuffix(strings.TrimPrefix(id, "{"), "}")
	raw = strings.ReplaceAll(raw, "-", "")
	if len(raw) != 32 {
		return "", errors.New("invalid account ID")
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return "", errors.New("invalid account ID")
	}
	raw = strings.ToLower(raw)
	return raw[:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:], nil
}
