package configplane

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"net"
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
