package reality

import (
	"errors"
	"net"
	"strings"
)

var ErrInvalidCandidate = errors.New("invalid reality candidate")

func ValidateCandidate(c Candidate) error {
	if c.Port < 1 || c.Port > 65535 || c.Target == "" || c.ServerName == "" {
		return ErrInvalidCandidate
	}
	for _, x := range []string{c.Target, c.ServerName} {
		if strings.ContainsAny(x, "/\\ \t\r\n") || net.ParseIP(x) != nil {
			return ErrInvalidCandidate
		}
	}
	return nil
}
