package configplane

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportcore"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

//go:embed runtime.py
var runtimeScript string

// The native panel can hot-add inbounds through Xray's API without changing its
// startup file. Verify the running process, its actual socket, active users and
// authenticated traffic, rather than mistaking the startup file for live state.
func (s Service) verifyRuntime(ctx context.Context, t provisioning.Target, key []byte, payload map[string]any) error {
	ssh, ok := s.SSH.(interface {
		RunInput(context.Context, provisioning.Target, []byte, string, []byte) (provisioning.CommandResult, error)
	})
	if !ok {
		return errors.New("profile_runtime_stdin_unavailable")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var obj map[string]any
	if json.Unmarshal(b, &obj) != nil {
		return errors.New("profile_runtime_payload_invalid")
	}
	settings, ok := obj["settings"].(map[string]any)
	if !ok {
		return errors.New("profile_runtime_payload_invalid")
	}
	emails := []string{}
	clients, ok := settings["clients"].([]any)
	if !ok {
		return errors.New("profile_runtime_payload_invalid")
	}
	for _, v := range clients {
		c, ok := v.(map[string]any)
		if !ok {
			return errors.New("profile_runtime_payload_invalid")
		}
		email, ok := c["email"].(string)
		if !ok || email == "" {
			return errors.New("profile_runtime_payload_invalid")
		}
		emails = append(emails, email)
	}
	wanted := map[string]any{"protocol": obj["protocol"], "port": obj["port"], "tag": obj["tag"], "emails": emails, "probes": obj["probe_outbounds"], "probe_classes": obj["probe_classes"], "core_hashes": profileCoreHashes()}
	input, err := json.Marshal(wanted)
	if err != nil {
		return err
	}
	// Only the fixed program is in argv; credentials stay in encrypted SSH stdin.
	command := "python3 -c \"import base64;exec(base64.b64decode('" + base64.StdEncoding.EncodeToString([]byte(runtimeScript)) + "'))\""
	out, err := ssh.RunInput(ctx, t, key, command, input)
	var result struct {
		Verified bool
		Reason   string
	}
	decodeErr := json.Unmarshal([]byte(out.Stdout), &result)
	if err != nil || decodeErr != nil || !result.Verified {
		switch result.Reason {
		case "binary", "socket", "users", "traffic":
			return errors.New("profile_runtime_" + result.Reason + "_unverified")
		}
		return errors.New("profile_runtime_unverified")
	}
	// A node-local successful client does not prove its cloud firewall admits
	// the published endpoint. Require reachability from the controller too.
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, e := (&net.Dialer{}).DialContext(probe, "tcp", net.JoinHostPort(t.Host, fmt.Sprint(payload["port"])))
	if e != nil {
		return errors.New("profile_public_port_unreachable")
	}
	_ = conn.Close()
	return nil
}

// Official v26.9.9 assets are pinned alongside the installed compatibility core.
// Their archives were verified against the release .dgst files before deriving
// these executable hashes. Unknown binaries still cannot authorize publication.
func profileCoreHashes() []string {
	return append(transportcore.CoreHashes(),
		"c4ae6798c38e0e5343b192406746333cd0ba7ff3eb984f8c4b9939dcb68c3f8a",
		"c1defe42b6db958a97c5e049a02a00a4baaedaca7b51c1c229f0830e288acef5")
}
