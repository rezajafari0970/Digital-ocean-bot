package configplane

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"

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
	wanted := map[string]any{"protocol": obj["protocol"], "port": obj["port"], "tag": obj["tag"], "emails": emails, "probes": obj["probe_outbounds"], "core_hashes": transportcore.CoreHashes()}
	input, err := json.Marshal(wanted)
	if err != nil {
		return err
	}
	// Only the fixed program is in argv; credentials stay in encrypted SSH stdin.
	command := "python3 -c \"import base64;exec(base64.b64decode('" + base64.StdEncoding.EncodeToString([]byte(runtimeScript)) + "'))\""
	out, err := ssh.RunInput(ctx, t, key, command, input)
	var result struct{ Verified bool }
	if err != nil || json.Unmarshal([]byte(out.Stdout), &result) != nil || !result.Verified {
		return errors.New("profile_runtime_unverified")
	}
	return nil
}
