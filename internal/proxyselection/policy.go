// Package proxyselection orders candidates without owning routes, credentials,
// provider APIs, security exceptions or traffic. Drivers and the application
// supply eligibility and verified observations; adoption is a separate CAS.
package proxyselection

import (
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/geoctx"
	"strings"
)

const Auto = "AUTO"

// Country is the strict onboarding choice: AUTO or exactly one country.
func Country(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return Auto, nil
	}
	values, err := Normalize([]string{raw})
	if err != nil {
		return "", err
	}
	return values[0], nil
}

func Normalize(countries []string) ([]string, error) {
	if len(countries) == 0 {
		return []string{Auto}, nil
	}
	if len(countries) > 251 {
		return nil, errors.New("too many country priorities")
	}
	result := make([]string, 0, len(countries))
	seen := map[string]bool{}
	for i, c := range countries {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c == Auto {
			if i != len(countries)-1 {
				return nil, errors.New("AUTO must be the last priority")
			}
		} else {
			c = geoctx.NormalizeCountryCode(c)
			if c == "" {
				return nil, errors.New("unknown country code")
			}
		}
		if seen[c] {
			return nil, errors.New("duplicate country priority")
		}
		seen[c] = true
		result = append(result, c)
	}
	return result, nil
}

func Legacy(countries []string) bool {
	return len(countries) == 0 || len(countries) == 1 && countries[0] == Auto
}
