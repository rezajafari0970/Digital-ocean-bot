package geoctx

import "testing"

func TestCoverage(t *testing.T) {
	if len(territoryDefaultLocale) < 200 {
		t.Fatalf("coverage too small: %d", len(territoryDefaultLocale))
	}
	for c, w := range map[string]string{"VE": "es-VE", "DE": "de-DE", "CA": "en-CA", "BE": "nl-BE", "CH": "de-CH", "IN": "hi-IN", "JP": "ja-JP", "BR": "pt-BR"} {
		if g := LocaleForCountry(c); g != w {
			t.Errorf("%s=%s want %s", c, g, w)
		}
	}
}
