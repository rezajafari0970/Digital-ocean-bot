package geoctx

import (
	"strings"
	"testing"
)

func TestCountryLabelsCoverISOAndUnicode(t *testing.T) {
	if len(countryEnglishNames) != 249 {
		t.Fatal("ISO coverage", len(countryEnglishNames))
	}
	seen := map[string]bool{}
	for _, code := range countryNameToCode {
		seen[strings.ToUpper(code)] = true
	}
	for code, name := range countryEnglishNames {
		if !seen[code] {
			t.Fatalf("code missing from independent ISO name index: %s", code)
		}
		label := CountryLabel(strings.ToLower(code))
		flag := []rune(strings.SplitN(label, " ", 2)[0])
		if len(flag) != 2 || flag[0] != 0x1F1E6+rune(code[0]-'A') || flag[1] != 0x1F1E6+rune(code[1]-'A') || !strings.HasSuffix(label, " "+name) {
			t.Fatal(code, label)
		}
	}
	for code, want := range map[string]string{"de": "🇩🇪 Germany", "GB": "🇬🇧 United Kingdom", "UK": "🇬🇧 United Kingdom", "US": "🇺🇸 United States", "CA": "🇨🇦 Canada", "KR": "🇰🇷 South Korea", "XK": "🇽🇰 Kosovo", "": "🌐 Unknown country", "ZZ": "🌐 Unknown country", "EU": "🌐 Unknown country", "A1": "🌐 Unknown country", "DE\nUS": "🌐 Unknown country"} {
		if got := CountryLabel(code); got != want {
			t.Fatal(code, got, want)
		}
	}
}
func BenchmarkCountryLabel(b *testing.B) {
	for b.Loop() {
		_ = CountryLabel("DE")
	}
}
