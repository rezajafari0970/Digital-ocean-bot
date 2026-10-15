package transportprofile_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
)

func TestClientMaskPreservesServerAndModernSchedules(t *testing.T) {
	c := transportprofile.Recommended()
	c.FinalMask = json.RawMessage(`{"tcp":[{"type":"fragment","settings":{"packets":"tlshello","lengths":["48-80",128,"288-448"],"delays":[0,"1-2",1],"maxSplit":"3-5"}},{"type":"fragment","settings":{"packets":"1-3","lengths":[200,400],"delays":["2-3",0],"maxSplit":3}}],"udp":[],"quicParams":{}}`)
	before := string(c.FinalMask)
	serverBefore, err := c.TCPMask()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := c.ClientTCPMask()
	if err != nil {
		t.Fatal(err)
	}
	again, err := c.ClientTCPMask()
	if err != nil || string(raw) != string(again) {
		t.Fatal("nondeterministic")
	}
	serverAfter, err := c.TCPMask()
	if err != nil || string(c.FinalMask) != before || string(serverBefore) != string(serverAfter) {
		t.Fatal("server mask or input mutated")
	}
	var original, client map[string]any
	json.Unmarshal(serverBefore, &original)
	json.Unmarshal(raw, &client)
	if len(client) != 1 {
		t.Fatal("inactive masks exported")
	}
	masks := client["tcp"].([]any)
	if len(masks) != 2 {
		t.Fatal("mask chain changed")
	}
	for i, mask := range masks {
		settings := mask.(map[string]any)["settings"].(map[string]any)
		expected := original["tcp"].([]any)[i].(map[string]any)["settings"].(map[string]any)
		if !reflect.DeepEqual(settings["length"], expected["lengths"].([]any)[0]) || !reflect.DeepEqual(settings["delay"], expected["delays"].([]any)[0]) {
			t.Fatal("wrong legacy scalar")
		}
		delete(settings, "length")
		delete(settings, "delay")
	}
	if !reflect.DeepEqual(client, original) {
		t.Fatal("modern schedule or other fields changed")
	}
}

func TestClientMaskRejectsInvalidSchedules(t *testing.T) {
	valid := `{"tcp":[{"type":"fragment","settings":{"packets":"tlshello","lengths":["48-80"],"delays":["0-1"],"maxSplit":3}}]}`
	for _, tc := range []struct{ name, old, replacement string }{
		{"empty-lengths", `["48-80"]`, `[]`},
		{"empty-delays", `["0-1"]`, `[]`},
		{"null-lengths", `["48-80"]`, `null`},
		{"null-delays", `["0-1"]`, `null`},
		{"null-length-entry", `["48-80"]`, `[null]`},
		{"null-delay-entry", `["0-1"]`, `[null]`},
		{"later-null-delay", `["0-1"]`, `[1,null]`},
		{"zero-length", `["48-80"]`, `[0]`},
		{"later-zero-length", `["48-80"]`, `[40,0]`},
		{"negative-length", `["48-80"]`, `["-1"]`},
		{"negative-delay", `["0-1"]`, `[-1]`},
		{"reversed", `["48-80"]`, `["80-48"]`},
		{"too-large", `["48-80"]`, `[16385]`},
		{"too-long-delay", `["0-1"]`, `[1001]`},
		{"wrong-type", `["48-80"]`, `[true]`},
		{"object", `["48-80"]`, `[{}]`},
		{"missing-lengths", `"lengths":["48-80"],`, ``},
		{"missing-delays", `"delays":["0-1"],`, ``},
		{"too-many", `["48-80"]`, `[` + strings.Repeat("48,", 64) + `48]`},
		{"unknown-field", `"packets":`, `"unexpected":1,"packets":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := transportprofile.Recommended()
			c.FinalMask = json.RawMessage(strings.Replace(valid, tc.old, tc.replacement, 1))
			raw, err := c.ClientTCPMask()
			if err == nil || len(raw) != 0 {
				t.Fatal("invalid mask exported")
			}
		})
	}
	for _, raw := range []string{`{`, `null`, `{}`, `{"tcp":[]}`} {
		c := transportprofile.Recommended()
		c.FinalMask = json.RawMessage(raw)
		if _, err := c.ClientTCPMask(); err == nil {
			t.Fatal("malformed mask accepted")
		}
	}
}
