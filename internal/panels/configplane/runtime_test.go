package configplane

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"net"
	"os/exec"
	"strings"
	"testing"
)

type runtimeProofSSH struct {
	t        *testing.T
	secret   string
	verified bool
}

func (s runtimeProofSSH) Run(context.Context, provisioning.Target, []byte, string) (string, error) {
	s.t.Fatal("credentials must not use command arguments")
	return "", nil
}
func (s runtimeProofSSH) RunInput(_ context.Context, _ provisioning.Target, _ []byte, command string, input []byte) (provisioning.CommandResult, error) {
	if strings.Contains(command, s.secret) {
		s.t.Fatal("credential exposed in argv")
	}
	var wanted map[string]any
	if json.Unmarshal(input, &wanted) != nil || wanted["tag"] != "fixture" || !strings.Contains(string(input), s.secret) {
		s.t.Fatal("missing encrypted-stdin proof")
	}
	classes, ok := wanted["probe_classes"].([]any)
	if !ok || len(classes) != 1 || classes[0] != "DIRECT" {
		s.t.Fatal("runtime proof lost client routing class")
	}
	raw, _ := json.Marshal(map[string]bool{"verified": s.verified})
	return provisioning.CommandResult{Stdout: string(raw)}, nil
}
func TestRuntimeProofRequiresPublicEndpointAndEncryptedStdin(t *testing.T) {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	credentials, e := configdriver.NewCredentials("profile", "panel", []string{"DIRECT"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	registry := configdriver.Builtins()
	spec := configdriver.Spec{Protocol: "shadowsocks", Network: "tcp", Security: "none", Port: port}
	payload, e := registry.Build(spec, "127.0.0.1", "fixture", credentials)
	if e != nil {
		t.Fatal(e)
	}
	outbound, e := registry.Outbound(spec, "127.0.0.1", credentials, credentials.Clients[0])
	if e != nil {
		t.Fatal(e)
	}
	payload["tag"] = "fixture"
	payload["probe_outbounds"] = []any{outbound}
	payload["probe_classes"] = []string{"DIRECT"}
	target := provisioning.Target{Host: "127.0.0.1"}
	service := Service{SSH: runtimeProofSSH{t, credentials.Clients[0].Password, true}}
	if e := service.verifyRuntime(context.Background(), target, nil, payload); e != nil {
		t.Fatal("valid proof", e)
	}
	service.SSH = runtimeProofSSH{t, credentials.Clients[0].Password, false}
	if e := service.verifyRuntime(context.Background(), target, nil, payload); e == nil {
		t.Fatal("failed native proof accepted")
	}
	service.SSH = runtimeProofSSH{t, credentials.Clients[0].Password, true}
	listener.Close()
	if e := service.verifyRuntime(context.Background(), target, nil, payload); e == nil {
		t.Fatal("unreachable published port accepted")
	}
}

// Reproduces the production route contract: a Residential client cannot use
// the Direct-only destination. Order comes from credentials, never a fixed index.
func TestRuntimeProbeMatchesRouteClass(t *testing.T) {
	script := `
import ast,pathlib
tree=ast.parse(pathlib.Path("runtime.py").read_text())
function=next(n for n in tree.body if isinstance(n,ast.FunctionDef) and n.name=="probe_targets")
scope={}
exec(compile(ast.Module(body=[function],type_ignores=[]),"runtime.py","exec"),scope)
targets=scope["probe_targets"]
allowed={"DIRECT":("example.com","/"),"RESIDENTIAL":("browserleaks.com","/ip")}
for classes in (["DIRECT"],["RESIDENTIAL"],["DIRECT","RESIDENTIAL"],["RESIDENTIAL","DIRECT"]):
 actual=targets([{} for _ in classes],classes)
 for cls,target in zip(classes,actual):
  assert target==allowed[cls],(cls,target)
for probes,classes in (([],[]),([{}],[]),([{}],["unknown"]),([{},{}],["DIRECT"]),([{},{},{}],["DIRECT"]*3)):
 try: targets(probes,classes)
 except ValueError: pass
 else: raise AssertionError("invalid probe metadata accepted")
`
	if output, err := exec.Command("python3", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("class-aware proof: %v: %s", err, output)
	}
}

func TestProfileRuntimeAcceptsPinnedOfficialAndCompatibilityCores(t *testing.T) {
	hashes := profileCoreHashes()
	seen := map[string]bool{}
	for _, h := range hashes {
		if len(h) != 64 || seen[h] {
			t.Fatal("invalid or duplicate core pin")
		}
		seen[h] = true
	}
	for _, h := range []string{"c4ae6798c38e0e5343b192406746333cd0ba7ff3eb984f8c4b9939dcb68c3f8a", "c1defe42b6db958a97c5e049a02a00a4baaedaca7b51c1c229f0830e288acef5"} {
		if !seen[h] {
			t.Fatal("official core excluded")
		}
	}
	if len(hashes) != 4 {
		t.Fatal("unexpected trust expansion")
	}
}
