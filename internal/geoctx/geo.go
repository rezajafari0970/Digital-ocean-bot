package geoctx

import "strings"

func LocaleForCountry(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if v := territoryDefaultLocale[code]; v != "" {
		return v
	}
	return "en-US"
}

func CountryCodeForName(name string) string {
	v := strings.ToLower(strings.TrimSpace(name))
	v = strings.Join(strings.Fields(v), " ")
	if len(v) == 2 {
		if territoryDefaultLocale[strings.ToUpper(v)] != "" {
			return v
		}
	}
	return countryNameToCode[v]
}
