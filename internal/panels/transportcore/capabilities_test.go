package transportcore

import (
	"os/exec"
	"strings"
	"testing"
)

func TestRuntimeRejectsMissingFragmentCapabilities(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 is required for the native installer regression", err)
	}
	definitions, _, ok := strings.Cut(installer, "\ntry:\n main(SPEC)")
	if !ok {
		t.Fatal("installer entrypoint missing")
	}
	program := definitions + `
required=['websocket_graceful_close','tls_certificate_snapshot','reality_fallback_bounded','reality_handshake_deadline','fragmented_http_sniffing','fragmented_http_header','fragmented_trojan_auth']
caps=dict.fromkeys(required,True)
caps.update(schema=1,reality_fallback_max_connections=512,reality_finalmask_fragment=True)
validate_capabilities(caps,False)
validate_capabilities(caps,True)
for key in required+['schema','reality_fallback_max_connections','reality_finalmask_fragment']:
 bad=dict(caps)
 del bad[key]
 try:
  validate_capabilities(bad,True)
 except RuntimeError as exc:
  assert str(exc)=='core_capability_missing'
 else:
  raise AssertionError('missing capability accepted: '+key)
for key in required:
 bad=dict(caps)
 bad[key]=1
 try:
  validate_capabilities(bad,False)
 except RuntimeError:
  pass
 else:
  raise AssertionError('nonboolean capability accepted: '+key)
`
	if out, err := exec.Command(python, "-c", program).CombinedOutput(); err != nil {
		t.Fatalf("installer accepted an incompatible runtime: %s %v", out, err)
	}
}
