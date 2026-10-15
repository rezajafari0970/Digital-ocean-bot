package transportapply

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRuntimeVerifierRejectsDroppedUsers(t *testing.T) {
	python, e := exec.LookPath("python3")
	if e != nil {
		t.Skip("python3 required")
	}
	dir := t.TempDir()
	core := filepath.Join(dir, "core")
	want := map[string]any{"port": 443, "network": "xhttp", "security": "reality", "flow": "", "decryption_hash": digest("none"), "private_hash": digest(""), "shortIds": []string{"1234"}, "serverNames": []string{"fixture.example"}, "clients": []string{digest("kept\x00kept")}}
	config := map[string]any{"inbounds": []any{map[string]any{"protocol": "vless", "port": 443, "settings": map[string]any{"decryption": "none", "clients": []any{}}, "streamSettings": map[string]any{"network": "xhttp", "security": "reality", "realitySettings": map[string]any{"shortIds": []string{"1234"}, "serverNames": []string{"fixture.example"}}}}}}
	raw, _ := json.Marshal(config)
	if e = os.WriteFile(filepath.Join(dir, "config.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	spec, _ := json.Marshal(want)
	script := strings.ReplaceAll(runtimeVerifier, "/usr/local/x-ui/bin/xray-linux-amd64", core)
	script = strings.ReplaceAll(script, "range(4)", "range(1)")
	script = strings.ReplaceAll(script, "time.sleep(1)", "pass")
	out, e := exec.Command(python, "-c", "import json,hashlib,pathlib,subprocess,os,time\nwant=json.loads("+strconv.Quote(string(spec))+")\n"+script).CombinedOutput()
	if e == nil || !strings.Contains(string(out), `"reason":"client_membership"`) {
		t.Fatalf("missing runtime users accepted: %s %v", out, e)
	}
}
func TestVerifyRejectsUnrelatedMetadataLoss(t *testing.T) {
	raw, old := fixture(t)
	m, _ := decode(raw)
	m["allocate"] = map[string]any{"strategy": "always"}
	settings, _ := object(m["settings"])
	settings["extension"] = map[string]any{"keep": true}
	m["settings"] = settings
	raw, _ = json.Marshal(m)
	target, _ := Target(old, transportprofile.Recommended())
	after, _, e := Transform(raw, testKey, target)
	if e != nil {
		t.Fatal(e)
	}
	for _, field := range []string{"allocate", "extension", "fallbacks"} {
		obj, _ := decode(after)
		if field == "fallbacks" {
			s, _ := object(obj["settings"])
			s["fallbacks"] = []any{map[string]any{"dest": 80}}
			obj["settings"] = s
		} else if field == "extension" {
			s, _ := object(obj["settings"])
			delete(s, "extension")
			obj["settings"] = s
		} else {
			delete(obj, field)
		}
		changed, _ := json.Marshal(obj)
		if Verify(raw, changed, testKey, target) == nil {
			t.Fatal("metadata loss accepted", field)
		}
	}
}
