package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTimingRetainsKnownMilestonesWithoutSecrets(t *testing.T) {
	raw := []byte(`{"http_code":0,"time_namelookup":0.00003,"time_connect":0,"time_appconnect":0,"time_pretransfer":0,"time_starttransfer":0,"time_total":6.002599,"url":"https://secret.invalid/token","errormsg":"secret diagnostic","remote_ip":"192.0.2.1"}`)
	timing := parseTiming(raw)
	if timing == nil || timing.Total != 6.002599 || timing.Connect != 0 || timing.TLS != 0 {
		t.Fatal(timing)
	}
	encoded, _ := json.Marshal(timing)
	for _, forbidden := range []string{"secret", "url", "errormsg", "remote_ip", "192.0.2.1"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("sensitive field retained", forbidden)
		}
	}
}
func TestTimingMissingMalformedOrImpossibleRemainsUnknown(t *testing.T) {
	valid := `{"time_namelookup":0.001,"time_connect":0.002,"time_appconnect":0.3,"time_pretransfer":0.31,"time_starttransfer":0.4,"time_total":0.5}`
	if parseTiming([]byte(valid)) == nil {
		t.Fatal("valid trace lost")
	}
	for _, raw := range []string{`{}`, `null`, `[]`, `{"http_code":204}`, strings.Replace(valid, `"time_total":0.5`, `"time_total":null`, 1), strings.Replace(valid, `"time_total":0.5`, `"time_total":"0.5"`, 1), strings.Replace(valid, `"time_total":0.5`, `"time_total":-1`, 1), strings.Replace(valid, `"time_total":0.5`, `"time_total":1e999`, 1), strings.Replace(valid, `"time_total":0.5`, `"time_total":61`, 1), strings.Replace(valid, `"time_total":0.5`, `"time_total":0.2`, 1)} {
		if parseTiming([]byte(raw)) != nil {
			t.Fatal("invalid timing accepted", raw)
		}
	}
}
func TestTimeoutKeepsVerdictAndAvailableMilestones(t *testing.T) {
	d := t.TempDir()
	script := `#!/bin/sh
printf '%s' '{"http_code":0,"time_namelookup":0.00003,"time_connect":0.0001,"time_appconnect":0,"time_pretransfer":0,"time_starttransfer":0,"time_total":6.002}'
exit 28
`
	if err := os.WriteFile(filepath.Join(d, "curl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d)
	got := attempt(context.Background(), 1080, "https://example.invalid", 204)
	if got.Outcome != "timeout" || got.CurlCode != 28 || got.HTTPStatus != 0 || got.Timing == nil || got.Timing.TLS != 0 || got.Timing.Total != 6.002 {
		t.Fatal(got)
	}
}

func TestUnobservedTimingAndImpossiblePositiveMilestoneAreOmitted(t *testing.T) {
	for _, raw := range []string{
		`{"time_namelookup":0,"time_connect":0,"time_appconnect":0,"time_pretransfer":0,"time_starttransfer":0,"time_total":0}`,
		`{"time_namelookup":0.0005,"time_connect":0,"time_appconnect":0,"time_pretransfer":0,"time_starttransfer":0,"time_total":0}`,
		`{"time_namelookup":0,"time_connect":0.5005,"time_appconnect":0,"time_pretransfer":0,"time_starttransfer":0,"time_total":0.5}`,
	} {
		d := t.TempDir()
		script := "#!/bin/sh\nprintf '%s' '" + raw + "'\nexit 28\n"
		if err := os.WriteFile(filepath.Join(d, "curl"), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", d)
		got := attempt(context.Background(), 1080, "https://example.invalid", 204)
		if got.Timing != nil || got.Outcome != "timeout" || got.CurlCode != 28 || got.HTTPStatus != 0 {
			t.Fatal("unknown timing changed failure", got)
		}
		encoded, _ := json.Marshal(got)
		if strings.Contains(string(encoded), "timing") {
			t.Fatal("unobserved timing changed legacy envelope", string(encoded))
		}
	}
}
