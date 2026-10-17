package geoctx

import "strings"

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
