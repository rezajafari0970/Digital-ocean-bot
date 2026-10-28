package geoctx

import (
	"sort"
	"strings"
)

type CountryOption struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// CountryOptions lists the entire local ISO catalogue, independently of any
// provider's current coverage. Availability must be reported separately.
func CountryOptions() []CountryOption {
	codes := make([]string, 0, len(countryEnglishNames)+1)
	for code := range countryEnglishNames {
		codes = append(codes, code)
	}
	codes = append(codes, "XK")
	sort.Strings(codes)
	out := make([]CountryOption, 0, len(codes))
	for _, code := range codes {
		out = append(out, CountryOption{code, CountryLabel(code)})
	}
	return out
}

// NormalizeCountryCode accepts assigned ISO 3166-1 territories and the explicit
// UK alias used by UpCloud. XK is Unicode's Kosovo flag extension, not ISO.
func NormalizeCountryCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "UK" {
		code = "GB"
	}
	if countryEnglishNames[code] != "" || code == "XK" {
		return code
	}
	return ""
}

// CountryLabel is entirely local. Unknown metadata never becomes a guessed flag.
func CountryLabel(code string) string {
	code = NormalizeCountryCode(code)
	if code == "" {
		return "🌐 Unknown country"
	}
	name := countryEnglishNames[code]
	if code == "XK" {
		name = "Kosovo"
	}
	flag := string([]rune{0x1F1E6 + rune(code[0]-'A'), 0x1F1E6 + rune(code[1]-'A')})
	return flag + " " + name
}
