package geoctx

import "strings"

func LocaleForCountry(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if v := territoryDefaultLocale[code]; v != "" {
		return v
	}
	return "en-US"
}
