package geoctx

import "testing"

func TestCountryCodeForName(t *testing.T) {
	tests := map[string]string{
		"United Kingdom": "gb",
		"Spain":          "es",
		"Venezuela":      "ve",
		"Iran":           "ir",
		"DE":             "de",
	}
	for in, want := range tests {
		if got := CountryCodeForName(in); got != want {
			t.Fatalf("%q got=%q want=%q", in, got, want)
		}
	}
}
