package advancedreality

import (
	"errors"
	"strings"
	"testing"
)

func TestRealityInboundMaskGuard(t *testing.T) {
	cases := []struct {
		name, raw string
		err       error
	}{
		{"nested_case_alias", `{"Inbounds":[{"StreamSettings":{"Security":"reality","FinalMask":{"TCP":[{}]}}}]}`, ErrTransportCombination},
		{"ambiguous_security", `{"inbounds":[{"streamSettings":{"security":"tls","Security":"reality"}}]}`, ErrJSON},
		{"known_crash", `{"inbounds":[{"streamSettings":{"security":"reality","finalmask":{"tcp":[{"type":"fragment"}]}}}]}`, ErrTransportCombination},
		{"server_alias", `{"inbounds":[{"streamSettings":{"security":"reality","serverFinalMask":{"tcp":[{}]}}}]}`, ErrTransportCombination},
		{"case_alias", `{"inbounds":[{"streamSettings":{"security":"reality","FinalMask":{"TCP":[{}]}}}]}`, ErrTransportCombination},
		{"empty", `{"inbounds":[{"streamSettings":{"security":"reality","finalmask":{"tcp":[]}}}]}`, nil},
		{"tls", `{"inbounds":[{"streamSettings":{"security":"tls","finalmask":{"tcp":[{}]}}}]}`, nil},
		{"outbound_unchanged", `{"outbounds":[{"streamSettings":{"security":"reality","finalmask":{"tcp":[{}]}}}]}`, nil},
		{"mask_wrong_shape", `{"inbounds":[{"streamSettings":{"security":"reality","finalmask":"DO_NOT_ECHO"}}]}`, ErrTransportCombination},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, e := Inspect([]byte(tt.raw), []byte(tt.raw), "26.9.9")
			if !errors.Is(e, tt.err) {
				t.Fatalf("got %v want %v", e, tt.err)
			}
			if e != nil && strings.Contains(e.Error(), "DO_NOT_ECHO") {
				t.Fatal("sensitive error")
			}
		})
	}
}
