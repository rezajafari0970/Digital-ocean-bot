package protocolprobe

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestProtocolEvidenceBindingAndIsolation(t *testing.T) {
	o := map[string]any{"protocol": "socks", "settings": map[string]any{"password": "fixture-one"}}
	original := Fingerprint(DriverSOCKS5, o)
	o["settings"].(map[string]any)["password"] = "fixture-two"
	if original == Fingerprint(DriverSOCKS5, o) {
		t.Fatal("credential change retained evidence")
	}
	candidates := []Candidate{{Driver: DriverSOCKS5, ID: "one", Digest: original}}
	good := Observation{Driver: DriverSOCKS5, ID: "one", Digest: original, DNS: true, QUIC: true, Code: "ok"}
	encode := func(rows []Observation) string {
		b, _ := json.Marshal(map[string]any{"results": rows})
		return string(b)
	}
	if _, e := Decode(encode([]Observation{good}), candidates); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*Observation){
		func(o *Observation) { o.ID = "other" },
		func(o *Observation) { o.Digest = strings.Repeat("f", 64) },
		func(o *Observation) { o.QUIC = false },
		func(o *Observation) { o.DNS = false },
		func(o *Observation) { o.Code = "https_ok" },
	} {
		bad := good
		change(&bad)
		if _, e := Decode(encode([]Observation{bad}), candidates); e == nil {
			t.Fatal("invalid evidence accepted", bad)
		}
	}
	for _, raw := range []string{"{}", encode(nil), encode([]Observation{good, good}), strings.Repeat(" ", 65537)} {
		if _, e := Decode(raw, candidates); e == nil {
			t.Fatal("partial/oversized evidence accepted")
		}
	}
}

func TestProtocolProbeResponseValidation(t *testing.T) {
	if out, e := exec.Command("python3", "probe_test.py", "validation").CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, out)
	}
}
func TestNativeProtocolProbeCapability(t *testing.T) {
	if os.Getenv("XRAY_TEST_BINARY") == "" {
		t.Skip("pinned native core required")
	}
	if out, e := exec.Command("python3", "probe_test.py", "native").CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, out)
	}
}
