package transportprofile

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const scalarProfileMask = `{"tcp":[{"type":"fragment","settings":{"packets":"tlshello","maxSplit":"0","length":"1-104","delay":"0"}},{"type":"fragment","settings":{"packets":"1-1","maxSplit":"11","length":"1-114","delay":"1"}}],"udp":[{"type":"header-custom","settings":{"client":[{"rand":16,"randRange":"0-255"}],"server":[{"rand":16,"randRange":"0-255"}]}}]}`

func TestProfileMaskPreservesScalarChainAndUDP(t *testing.T) {
	for _, input := range []string{scalarProfileMask, strings.ReplaceAll(scalarProfileMask, `"maxSplit":"0"`, `"maxSplit":0`)} {
		c := Config{FinalMask: json.RawMessage(input)}
		result, e := c.ProfileMask()
		if e != nil {
			t.Fatal(e)
		}
		var want, got any
		json.Unmarshal([]byte(input), &want)
		json.Unmarshal(result, &got)
		if !reflect.DeepEqual(want, got) || string(c.FinalMask) != input {
			t.Fatal("operator mask changed or UDP lost")
		}
		server, e := c.TCPMask()
		if e != nil || strings.Contains(string(server), `"udp"`) {
			t.Fatal("server-only legacy contract changed", e)
		}
	}
	if _, e := (Config{FinalMask: json.RawMessage(ActiveFinalMaskPreset)}).ProfileMask(); e != nil {
		t.Fatal("array preset regressed", e)
	}
}

func TestProfileMaskRejectsMalformedHeadersAndNulls(t *testing.T) {
	for _, tc := range []struct{ from, to string }{
		{`"delay":"0"`, `"delay":null`}, {`"maxSplit":"0"`, `"maxSplit":null`},
		{`"length":"1-104"`, `"length":"0-104"`}, {`"rand":16`, `"rand":null`},
		{`"rand":16`, `"rand":-1`}, {`"rand":16`, `"rand":8193`},
		{`"randRange":"0-255"`, `"randRange":"0-256"`},
		{`"rand":16`, `"rand":16,"packet":[1]`}, {`"rand":16`, `"packet":[null]`},
		{`"rand":16`, `"type":"hex","packet":"zz"`},
		{`"type":"header-custom"`, `"type":"typo"`},
		{`"client":`, `"clients":`}, {`"tcp":`, `"typo":`},
	} {
		raw := strings.Replace(scalarProfileMask, tc.from, tc.to, 1)
		if _, e := (Config{FinalMask: json.RawMessage(raw)}).ProfileMask(); e == nil {
			t.Fatalf("accepted %s", tc.to)
		}
	}
}
